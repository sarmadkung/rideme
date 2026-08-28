/**
 * The platform's typed API client — CAP-6's shared client infrastructure.
 *
 * Every type it returns is generated from the Go handlers that serve it
 * (ADR-007), so a client cannot describe a response the server does not send.
 * Transport concerns — base URL, auth, refresh, error mapping, idempotency —
 * are settled once here so no screen re-invents them, and no *product* logic
 * lives here at all: this package knows how to call the API and nothing about
 * what the answers mean.
 */
import type {
  ApiErrorBody,
  CancelResult,
  ErrorCode,
  HealthResponse,
  Job,
  Quote,
} from '@platform/types';
import {
  apiErrorBodySchema,
  cancelResultSchema,
  healthResponseSchema,
  jobSchema,
  quoteSchema,
} from '@platform/validation';

export class ApiError extends Error {
  readonly code: ErrorCode;
  readonly status: number;
  readonly requestId: string;
  readonly details: Readonly<Record<string, string>>;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = body.code;
    this.requestId = body.request_id;
    this.details = body.details ?? {};
  }

  /** A refresh will not help; the caller must log in again. */
  get requiresLogin(): boolean {
    return this.code === 'unauthorized';
  }

  /** Worth retrying after a delay — the server said so. */
  get retryable(): boolean {
    return this.code === 'rate_limited' || this.code === 'unavailable';
  }
}

/**
 * Where refresh credentials live.
 *
 * Document 28 requires secure platform storage — iOS Keychain, Android
 * Keystore — and explicitly forbids plain AsyncStorage. This package cannot
 * reach either, so it takes storage as an interface and each app supplies the
 * right one. That also keeps the client testable without a device.
 */
export interface TokenStorage {
  get(): Promise<string | null>;
  set(token: string): Promise<void>;
  clear(): Promise<void>;
}

/** An in-memory store, for tests and for web sessions that should not persist. */
export function memoryTokenStorage(initial: string | null = null): TokenStorage {
  let token = initial;
  return {
    get: async () => token,
    set: async (value) => {
      token = value;
    },
    clear: async () => {
      token = null;
    },
  };
}

export interface Session {
  accessToken: string;
  expiresAt: Date;
}

export interface ApiClientOptions {
  baseUrl: string;
  /** Injected so tests and React Native can supply their own. */
  fetch?: typeof globalThis.fetch;
  timeoutMs?: number;
  storage?: TokenStorage;
  /** Device signals the server records for session trust (document 116). */
  device?: { id?: string; platform?: string; os?: string; appVersion?: string };
  /** Called when the session ends and the user must log in again. */
  onSessionExpired?: () => void;
}

const DEFAULT_TIMEOUT_MS = 15_000;

/** The API surface documents 14 and 35 define. */
export interface ApiClient {
  health(): Promise<HealthResponse>;

  requestOtp(phone: string): Promise<{ expiresAt: Date }>;
  verifyOtp(phone: string, code: string): Promise<Session>;
  logout(): Promise<void>;
  isAuthenticated(): boolean;

  quote(input: QuoteInput): Promise<Quote>;
  createJob(input: CreateJobInput, idempotencyKey: string): Promise<Job>;
  listJobs(options?: {
    limit?: number;
    cursor?: string;
  }): Promise<{ items: Job[]; nextCursor?: string }>;
  getJob(id: string): Promise<Job>;
  cancelJob(id: string, reason?: string): Promise<CancelResult>;

  driverArrive(jobId: string): Promise<Job>;
  driverStart(jobId: string): Promise<Job>;
  driverComplete(jobId: string): Promise<Job>;
}

export interface StopInput {
  type?: 'PICKUP' | 'DROPOFF' | 'WAYPOINT';
  latitude: number;
  longitude: number;
  address?: string;
  contactName?: string;
  contactPhone?: string;
}

export interface QuoteInput {
  jobType: string;
  vehicleType: string;
  city?: string;
  stops: StopInput[];
  requirements?: Record<string, string>;
}

export interface CreateJobInput {
  quoteId: string;
  jobType: string;
  stops: StopInput[];
  requirements?: Record<string, string>;
  scheduledAt?: Date;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** Skips the Authorization header, for the endpoints that create a session. */
  anonymous?: boolean;
  idempotencyKey?: string;
  query?: Record<string, string | number | undefined>;
}

export function createApiClient(options: ApiClientOptions): ApiClient {
  const baseUrl = options.baseUrl.replace(/\/+$/, '') + '/api/v1';
  const doFetch = options.fetch ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const storage = options.storage ?? memoryTokenStorage();

  let session: Session | null = null;
  /**
   * Concurrent 401s share one refresh. Without this, a screen that fires five
   * requests on mount performs five refreshes — and since refresh tokens
   * rotate, four of them present a token the server has already retired, which
   * it correctly treats as theft and responds to by ending every session.
   */
  let refreshInFlight: Promise<boolean> | null = null;

  function deviceHeaders(): Record<string, string> {
    const device = options.device;
    if (!device) return {};
    const headers: Record<string, string> = {};
    if (device.id) headers['X-Device-Id'] = device.id;
    if (device.platform) headers['X-Platform'] = device.platform;
    if (device.os) headers['X-OS'] = device.os;
    if (device.appVersion) headers['X-App-Version'] = device.appVersion;
    return headers;
  }

  async function send(path: string, opts: RequestOptions = {}): Promise<unknown> {
    const url = new URL(baseUrl + path);
    for (const [key, value] of Object.entries(opts.query ?? {})) {
      if (value !== undefined && value !== '') url.searchParams.set(key, String(value));
    }

    const headers: Record<string, string> = {
      accept: 'application/json',
      ...deviceHeaders(),
    };
    if (opts.body !== undefined) headers['content-type'] = 'application/json';
    if (opts.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;
    if (!opts.anonymous && session) headers['authorization'] = `Bearer ${session.accessToken}`;

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    let response: Response;
    try {
      response = await doFetch(url.toString(), {
        method: opts.method ?? 'GET',
        headers,
        signal: controller.signal,
        ...(opts.body !== undefined ? { body: JSON.stringify(opts.body) } : {}),
      });
    } catch (cause) {
      // A network failure is reported in the platform's own error shape, so
      // callers handle one kind of error rather than two.
      throw new ApiError(0, {
        code: 'unavailable',
        message: cause instanceof Error ? cause.message : 'network request failed',
        request_id: '',
      });
    } finally {
      clearTimeout(timer);
    }

    if (response.status === 204) return null;

    const payload: unknown = await response.json().catch(() => null);

    if (!response.ok) {
      const parsed = apiErrorBodySchema.safeParse(payload);
      const body: ApiErrorBody = parsed.success
        ? parsed.data
        : {
            code: 'internal',
            message: `unexpected ${response.status} response`,
            request_id: response.headers.get('x-request-id') ?? '',
          };
      throw new ApiError(response.status, body);
    }
    return payload;
  }

  /** Sends, and retries once through a refresh when the access token expired. */
  async function request(path: string, opts: RequestOptions = {}): Promise<unknown> {
    try {
      return await send(path, opts);
    } catch (error) {
      if (!(error instanceof ApiError) || error.status !== 401 || opts.anonymous) throw error;

      const refreshed = await refresh();
      if (!refreshed) {
        options.onSessionExpired?.();
        throw error;
      }
      return send(path, opts);
    }
  }

  async function refresh(): Promise<boolean> {
    if (refreshInFlight) return refreshInFlight;

    refreshInFlight = (async () => {
      const token = await storage.get();
      if (!token) return false;
      try {
        const body = (await send('/auth/refresh', {
          method: 'POST',
          anonymous: true,
          body: { refresh_token: token },
        })) as { access_token: string; refresh_token: string; expires_at: string };

        session = { accessToken: body.access_token, expiresAt: new Date(body.expires_at) };
        // The server rotates the refresh token on every use, so the new one
        // must be stored before anything else can fail.
        await storage.set(body.refresh_token);
        return true;
      } catch {
        // The session is gone — expired, revoked, or the token was reused
        // elsewhere and the server ended every session.
        await storage.clear();
        session = null;
        return false;
      } finally {
        refreshInFlight = null;
      }
    })();

    return refreshInFlight;
  }

  return {
    async health() {
      return healthResponseSchema.parse(await send('/../health', { anonymous: true }));
    },

    async requestOtp(phone) {
      const body = (await request('/auth/otp/request', {
        method: 'POST',
        anonymous: true,
        body: { phone },
      })) as { expires_at: string };
      return { expiresAt: new Date(body.expires_at) };
    },

    async verifyOtp(phone, code) {
      const body = (await request('/auth/otp/verify', {
        method: 'POST',
        anonymous: true,
        body: { phone, code },
      })) as { access_token: string; refresh_token: string; expires_at: string };

      session = { accessToken: body.access_token, expiresAt: new Date(body.expires_at) };
      await storage.set(body.refresh_token);
      return session;
    },

    async logout() {
      try {
        await request('/auth/logout', { method: 'POST' });
      } finally {
        // Local state is cleared even if the call fails: a user who taps log
        // out must be logged out of this device regardless.
        await storage.clear();
        session = null;
      }
    },

    isAuthenticated() {
      return session !== null;
    },

    async quote(input) {
      return quoteSchema.parse(
        await request('/quotes', {
          method: 'POST',
          body: {
            job_type: input.jobType,
            vehicle_type: input.vehicleType,
            city: input.city,
            stops: input.stops.map(toStopBody),
            requirements: input.requirements,
          },
        }),
      );
    },

    async createJob(input, idempotencyKey) {
      return jobSchema.parse(
        await request('/jobs', {
          method: 'POST',
          // Document 14 requires this on job creation, and the caller supplies
          // it so a retry of the *same user action* reuses the same key.
          idempotencyKey,
          body: {
            quote_id: input.quoteId,
            job_type: input.jobType,
            stops: input.stops.map(toStopBody),
            requirements: input.requirements,
            scheduled_at: input.scheduledAt?.toISOString(),
          },
        }),
      );
    },

    async listJobs(listOptions = {}) {
      const body = (await request('/jobs', {
        query: { limit: listOptions.limit, cursor: listOptions.cursor },
      })) as { items: unknown[]; page?: { next_cursor?: string } };

      return {
        items: body.items.map((item) => jobSchema.parse(item)),
        ...(body.page?.next_cursor ? { nextCursor: body.page.next_cursor } : {}),
      };
    },

    async getJob(id) {
      return jobSchema.parse(await request(`/jobs/${encodeURIComponent(id)}`));
    },

    async cancelJob(id, reason) {
      return cancelResultSchema.parse(
        await request(`/jobs/${encodeURIComponent(id)}/cancel`, {
          method: 'POST',
          body: { reason: reason ?? '' },
        }),
      );
    },

    async driverArrive(jobId) {
      return jobSchema.parse(
        await request(`/driver/jobs/${encodeURIComponent(jobId)}/arrive`, { method: 'POST' }),
      );
    },
    async driverStart(jobId) {
      return jobSchema.parse(
        await request(`/driver/jobs/${encodeURIComponent(jobId)}/start`, { method: 'POST' }),
      );
    },
    async driverComplete(jobId) {
      return jobSchema.parse(
        await request(`/driver/jobs/${encodeURIComponent(jobId)}/complete`, { method: 'POST' }),
      );
    },
  };
}

function toStopBody(stop: StopInput) {
  return {
    type: stop.type ?? '',
    latitude: stop.latitude,
    longitude: stop.longitude,
    address: stop.address ?? '',
    contact_name: stop.contactName ?? '',
    contact_phone: stop.contactPhone ?? '',
  };
}
