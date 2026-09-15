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
import {
  openEventStream,
  type EventStream,
  type StreamHandlers,
  type StreamOptions,
} from './stream';
export {
  openEventStream,
  parseFrame,
  MAX_RETRY_MS,
  type EventStream,
  type StreamEvent,
  type StreamHandlers,
  type StreamOptions,
} from './stream';
import type {
  ApiErrorBody,
  CancelResult,
  DriverAssignment,
  DriverEarnings,
  DriverProfile,
  ErrorCode,
  GroceryOrder,
  HealthResponse,
  IssueAction,
  Job,
  LocationReport,
  MerchantOrder,
  MerchantQueue,
  OrderIssue,
  Place,
  Product,
  Quote,
  Store,
  SubstitutionPreference,
  Tariff,
  Zone,
} from '@platform/types';
import {
  apiErrorBodySchema,
  cancelResultSchema,
  driverAssignmentSchema,
  driverEarningsSchema,
  driverProfileSchema,
  groceryOrderSchema,
  healthResponseSchema,
  jobSchema,
  locationReportSchema,
  merchantOrderSchema,
  orderIssueSchema,
  placeSchema,
  productSchema,
  quoteSchema,
  storeSchema,
  tariffSchema,
  zoneSchema,
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
  /**
   * The roles this session's user holds. The server already sends these on
   * verify (internal/identity/handler.go's tokenResponse.user.roles) — a
   * screen that must gate on ADMIN/SUPER_ADMIN reads this rather than
   * decoding the access token itself, which is an opaque platform token, not
   * a JWT a client can parse (pkg/authn/token.go).
   */
  roles: string[];
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
  /** Injected so a test can drive the realtime stream without a network. */
  stream?: StreamOptions;
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

  /**
   * Watches a live job over the realtime gateway (documents 018, 047).
   *
   * Returns a handle to close. `onOpen` fires on every reconnection, and a
   * caller should refetch there rather than assume it resumed: the gateway is
   * explicitly not the system of record and does not replay what was missed.
   */
  watchJob(id: string, handlers: StreamHandlers): EventStream;
  cancelJob(id: string, reason?: string): Promise<CancelResult>;

  driverArrive(jobId: string): Promise<Job>;
  driverStart(jobId: string): Promise<Job>;
  driverComplete(jobId: string): Promise<Job>;
  driverAccept(jobId: string): Promise<Job>;
  driverReject(jobId: string): Promise<Job>;

  driverMe(): Promise<DriverProfile>;
  goOnline(at: PositionInput): Promise<DriverProfile>;
  goOffline(): Promise<DriverProfile>;
  reportLocation(fixes: PositionInput[]): Promise<LocationReport>;
  /** The offer or trip the driver is holding, or null when they hold nothing. */
  driverAssignment(): Promise<DriverAssignment | null>;

  /** Admin-only: service zones (document 97). The server enforces the role;
   * calling these as a non-admin fails with a 403 ApiError. */
  listZones(): Promise<Zone[]>;
  createZone(input: CreateZoneInput): Promise<Zone>;
  /** Admin-only: pricing configuration (documents 34, 142). */
  listTariffs(): Promise<Tariff[]>;
  createTariff(input: CreateTariffInput): Promise<Tariff>;

  /**
   * Places matching free text, biased toward `near` when it is known.
   *
   * Resolves to an empty array when nothing matched — that is an answer, and
   * the caller should say "no results", not "something went wrong". A provider
   * outage still throws, because the two must not look the same to a customer
   * who would otherwise retype their address until they gave up.
   */
  searchPlaces(query: string, near?: { latitude: number; longitude: number }): Promise<Place[]>;

  /** What the driver has made today and this week, and the trips behind it. */
  driverEarnings(): Promise<DriverEarnings>;

  /**
   * The merchant's fulfilment surface (document 72).
   *
   * Every one of these is scoped server-side to the merchant the caller
   * operates: the MERCHANT role says a shop is calling, not which shop, and
   * another shop's order must be as invisible as one that does not exist.
   */
  listMerchantOrders(options?: {
    queue?: MerchantQueue;
    limit?: number;
    cursor?: string;
  }): Promise<{ items: MerchantOrder[]; nextCursor?: string }>;
  getMerchantOrder(id: string): Promise<MerchantOrder>;
  acceptMerchantOrder(id: string): Promise<MerchantOrder>;
  /** A rejection carries a reason. The server refuses one without it. */
  rejectMerchantOrder(id: string, reason: string): Promise<MerchantOrder>;
  startPreparingMerchantOrder(id: string): Promise<MerchantOrder>;
  /** Ready hands the order to dispatch, which is why it can fail for reasons the shop cannot fix. */
  markMerchantOrderReady(id: string): Promise<MerchantOrder>;
  /**
   * An empty shelf, and what the shop proposes about it (document 74).
   *
   * What actually happens is decided by the customer's standing preference on
   * the line, server-side — so the returned issue's `action` may not be the
   * one that was proposed, and the caller must render what came back rather
   * than what it sent.
   */
  reportMerchantItemIssue(
    orderId: string,
    itemId: string,
    issue: ItemIssueInput,
  ): Promise<OrderIssue>;

  /**
   * The customer's side of grocery (documents 68, 71).
   *
   * A shop that is shut is still listed — `store.open` says so. A customer
   * looking for their usual kiryana at 3am needs to see that it is closed,
   * not that it has vanished, and the server is deliberate about that.
   */
  listStores(
    near: { latitude: number; longitude: number },
    options?: { radiusM?: number; limit?: number },
  ): Promise<Store[]>;
  storeCatalog(storeId: string, limit?: number): Promise<Product[]>;

  /** Opens a cart at one shop, or returns the one already open there. */
  openCart(storeId: string): Promise<GroceryOrder>;
  /** The cart back, not the line: the running total is the server's to compute. */
  addCartItem(orderId: string, item: CartItemInput): Promise<GroceryOrder>;
  /** Checkout. The address is asked for here because this is where it is needed. */
  placeGroceryOrder(orderId: string, delivery: DeliveryInput): Promise<GroceryOrder>;

  listGroceryOrders(limit?: number): Promise<GroceryOrder[]>;
  getGroceryOrder(id: string): Promise<GroceryOrder>;
  /**
   * Answers a substitution the shop asked about (BD-11).
   *
   * Accepting charges the substitute's price, up or down; declining loses the
   * line rather than restoring it, because the shelf is empty — which is why
   * they were asked. Either way the whole order comes back, because the answer
   * changed the total.
   */
  decideGroceryIssue(orderId: string, issueId: string, accept: boolean): Promise<GroceryOrder>;
  /** Their own order, and only while its state still allows it. */
  cancelGroceryOrder(orderId: string, reason?: string): Promise<GroceryOrder>;
}

/** One line a customer adds to a cart. */
export interface CartItemInput {
  productId: string;
  variantId?: string;
  quantity: number;
  /** What to do if the shelf is empty. The server's default applies when absent. */
  substitutionPreference?: SubstitutionPreference;
}

/** Where an order is going. */
export interface DeliveryInput {
  address: string;
  latitude: number;
  longitude: number;
  notes?: string;
}

/** What a picker reports when a line cannot be filled as ordered. */
export interface ItemIssueInput {
  reason: string;
  action: IssueAction;
  /** Required by the server when the action is a substitution, ignored otherwise. */
  substituteName?: string;
  substitutePriceMinor?: number;
}

/** One position report. Timestamped by the device, not the server. */
export interface PositionInput {
  latitude: number;
  longitude: number;
  accuracyM?: number;
  headingDeg?: number;
  speedMps?: number;
  jobId?: string;
  recordedAt?: Date;
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

export interface CreateZoneInput {
  name: string;
  city?: string;
  latitude: number;
  longitude: number;
  radiusMeters: number;
}

export interface CreateTariffInput {
  jobType: string;
  vehicleType?: string;
  city?: string;
  zoneId?: string;
  version: number;
  minimumFareMinor: number;
  baseMinor: number;
  perKmMinor: number;
  perMinuteMinor: number;
  waitingPerMinuteMinor?: number;
  loadingPerMinuteMinor?: number;
  perKgMinor?: number;
  serviceFeeMinor?: number;
  serviceFeeBps?: number;
  taxBps?: number;
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
  const streamOptions = options.stream ?? {};
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

        // Refresh rotates tokens; it does not change roles, and the refresh
        // response carries no user info to re-derive them from. Carry the
        // prior session's roles forward rather than losing them here.
        session = {
          accessToken: body.access_token,
          expiresAt: new Date(body.expires_at),
          roles: session?.roles ?? [],
        };
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
      })) as {
        access_token: string;
        refresh_token: string;
        expires_at: string;
        user: { roles: string[] };
      };

      session = {
        accessToken: body.access_token,
        expiresAt: new Date(body.expires_at),
        roles: body.user.roles,
      };
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

    watchJob(id, handlers) {
      // The token is read once, at subscribe time. A stream outliving its
      // access token reconnects and is refused with a 401, which surfaces
      // through onError; the caller falls back to fetching, which refreshes
      // through the normal path. Threading refresh into the stream would mean
      // two refresh paths racing each other, and the one that loses retires a
      // token the other is about to present.
      const url = `${baseUrl}/realtime?channel=` + encodeURIComponent(`job:${id}`);
      return openEventStream(url, session?.accessToken ?? null, handlers, streamOptions);
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
    async driverAccept(jobId) {
      return jobSchema.parse(
        await request(`/driver/jobs/${encodeURIComponent(jobId)}/accept`, { method: 'POST' }),
      );
    },
    async driverReject(jobId) {
      return jobSchema.parse(
        await request(`/driver/jobs/${encodeURIComponent(jobId)}/reject`, { method: 'POST' }),
      );
    },

    async driverMe() {
      return driverProfileSchema.parse(await request('/driver/me'));
    },
    async goOnline(at) {
      return driverProfileSchema.parse(
        await request('/driver/online', { method: 'POST', body: toPositionBody(at) }),
      );
    },
    async goOffline() {
      return driverProfileSchema.parse(await request('/driver/offline', { method: 'POST' }));
    },
    async reportLocation(fixes) {
      return locationReportSchema.parse(
        await request('/driver/location', {
          method: 'POST',
          body: { fixes: fixes.map(toPositionBody) },
        }),
      );
    },
    async searchPlaces(query, near) {
      try {
        const body = (await request('/places', {
          query: {
            q: query,
            lat: near?.latitude,
            lon: near?.longitude,
          },
        })) as { places?: unknown[] };
        return (body.places ?? []).map((place) => placeSchema.parse(place));
      } catch (error) {
        // Nothing matched. An empty list is the honest answer; an error here
        // would put a normal outcome on the app's failure path.
        if (error instanceof ApiError && error.status === 404) return [];
        throw error;
      }
    },
    async driverEarnings() {
      return driverEarningsSchema.parse(await request('/driver/earnings'));
    },
    async listMerchantOrders(listOptions = {}) {
      const body = (await request('/merchant/orders', {
        query: {
          queue: listOptions.queue,
          limit: listOptions.limit,
          cursor: listOptions.cursor,
        },
      })) as { items: unknown[]; page?: { next_cursor?: string } };

      return {
        items: body.items.map((item) => merchantOrderSchema.parse(item)),
        ...(body.page?.next_cursor ? { nextCursor: body.page.next_cursor } : {}),
      };
    },
    async getMerchantOrder(id) {
      return merchantOrderSchema.parse(await request(`/merchant/orders/${encodeURIComponent(id)}`));
    },
    async acceptMerchantOrder(id) {
      return merchantOrderSchema.parse(
        await request(`/merchant/orders/${encodeURIComponent(id)}/accept`, { method: 'POST' }),
      );
    },
    async rejectMerchantOrder(id, reason) {
      return merchantOrderSchema.parse(
        await request(`/merchant/orders/${encodeURIComponent(id)}/reject`, {
          method: 'POST',
          body: { reason },
        }),
      );
    },
    async startPreparingMerchantOrder(id) {
      return merchantOrderSchema.parse(
        await request(`/merchant/orders/${encodeURIComponent(id)}/preparing`, { method: 'POST' }),
      );
    },
    async markMerchantOrderReady(id) {
      return merchantOrderSchema.parse(
        await request(`/merchant/orders/${encodeURIComponent(id)}/ready`, { method: 'POST' }),
      );
    },
    async reportMerchantItemIssue(orderId, itemId, issue) {
      return orderIssueSchema.parse(
        await request(
          `/merchant/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/issue`,
          {
            method: 'POST',
            body: {
              reason: issue.reason,
              action: issue.action,
              substitute_name: issue.substituteName,
              substitute_price_minor: issue.substitutePriceMinor,
            },
          },
        ),
      );
    },
    async listStores(near, storeOptions = {}) {
      const body = (await request('/stores', {
        query: {
          lat: near.latitude,
          lon: near.longitude,
          radius_m: storeOptions.radiusM,
          limit: storeOptions.limit,
        },
      })) as { stores?: unknown[] };
      return (body.stores ?? []).map((store) => storeSchema.parse(store));
    },
    async storeCatalog(storeId, limit) {
      const body = (await request(`/stores/${encodeURIComponent(storeId)}/products`, {
        query: { limit },
      })) as { products?: unknown[] };
      return (body.products ?? []).map((product) => productSchema.parse(product));
    },
    async openCart(storeId) {
      return groceryOrderSchema.parse(
        await request('/orders', { method: 'POST', body: { store_id: storeId } }),
      );
    },
    async addCartItem(orderId, item) {
      return groceryOrderSchema.parse(
        await request(`/orders/${encodeURIComponent(orderId)}/items`, {
          method: 'POST',
          body: {
            product_id: item.productId,
            variant_id: item.variantId,
            quantity: item.quantity,
            substitution_preference: item.substitutionPreference,
          },
        }),
      );
    },
    async placeGroceryOrder(orderId, delivery) {
      return groceryOrderSchema.parse(
        await request(`/orders/${encodeURIComponent(orderId)}/place`, {
          method: 'POST',
          body: {
            delivery: {
              address: delivery.address,
              latitude: delivery.latitude,
              longitude: delivery.longitude,
              notes: delivery.notes,
            },
          },
        }),
      );
    },
    async listGroceryOrders(limit) {
      const body = (await request('/orders', { query: { limit } })) as { items: unknown[] };
      return body.items.map((item) => groceryOrderSchema.parse(item));
    },
    async getGroceryOrder(id) {
      return groceryOrderSchema.parse(await request(`/orders/${encodeURIComponent(id)}`));
    },
    async decideGroceryIssue(orderId, issueId, accept) {
      return groceryOrderSchema.parse(
        await request(
          `/orders/${encodeURIComponent(orderId)}/issues/${encodeURIComponent(issueId)}/decision`,
          { method: 'POST', body: { accept } },
        ),
      );
    },
    async cancelGroceryOrder(orderId, reason) {
      return groceryOrderSchema.parse(
        await request(`/orders/${encodeURIComponent(orderId)}/cancel`, {
          method: 'POST',
          body: { reason: reason ?? '' },
        }),
      );
    },
    async driverAssignment() {
      try {
        return driverAssignmentSchema.parse(await request('/driver/assignment'));
      } catch (error) {
        // Holding nothing is the normal state of an idle driver, not a
        // failure. Returning null keeps that out of the app's error path.
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },

    async listZones() {
      const items = (await request('/admin/zones')) as unknown[];
      return items.map((item) => zoneSchema.parse(item));
    },

    async createZone(input) {
      return zoneSchema.parse(
        await request('/admin/zones', {
          method: 'POST',
          body: {
            name: input.name,
            city: input.city,
            latitude: input.latitude,
            longitude: input.longitude,
            radius_meters: input.radiusMeters,
          },
        }),
      );
    },

    async listTariffs() {
      const items = (await request('/admin/pricing/tariffs')) as unknown[];
      return items.map((item) => tariffSchema.parse(item));
    },

    async createTariff(input) {
      return tariffSchema.parse(
        await request('/admin/pricing/tariffs', {
          method: 'POST',
          body: {
            job_type: input.jobType,
            vehicle_type: input.vehicleType,
            city: input.city,
            zone_id: input.zoneId,
            version: input.version,
            minimum_fare_minor: input.minimumFareMinor,
            base_minor: input.baseMinor,
            per_km_minor: input.perKmMinor,
            per_minute_minor: input.perMinuteMinor,
            waiting_per_minute_minor: input.waitingPerMinuteMinor ?? 0,
            loading_per_minute_minor: input.loadingPerMinuteMinor ?? 0,
            per_kg_minor: input.perKgMinor ?? 0,
            service_fee_minor: input.serviceFeeMinor ?? 0,
            service_fee_bps: input.serviceFeeBps ?? 0,
            tax_bps: input.taxBps ?? 0,
          },
        }),
      );
    },
  };
}

function toPositionBody(position: PositionInput) {
  return {
    latitude: position.latitude,
    longitude: position.longitude,
    accuracy_m: position.accuracyM,
    heading_deg: position.headingDeg,
    speed_mps: position.speedMps,
    job_id: position.jobId,
    // The device's own clock. Document 048 is explicit that a fix is timed by
    // when it was recorded, not by when the server happened to receive it.
    recorded_at: (position.recordedAt ?? new Date()).toISOString(),
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
