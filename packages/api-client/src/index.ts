/**
 * Typed API client (document 23).
 *
 * Phase 1 exposes only what the Go API actually serves: health. Domain calls
 * arrive with their own slices. Transport concerns — base URL, error mapping,
 * request IDs — are settled once, here, so no slice re-invents them.
 */
import type { ApiErrorBody, ErrorCode, HealthResponse } from '@platform/types';
import { apiErrorBodySchema, healthResponseSchema } from '@platform/validation';

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
}

export interface ApiClientOptions {
  baseUrl: string;
  /** Injected so tests and React Native can supply their own. */
  fetch?: typeof globalThis.fetch;
  timeoutMs?: number;
}

export interface ApiClient {
  health(): Promise<HealthResponse>;
}

const DEFAULT_TIMEOUT_MS = 10_000;

export function createApiClient(options: ApiClientOptions): ApiClient {
  const baseUrl = options.baseUrl.replace(/\/+$/, '');
  const doFetch = options.fetch ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;

  async function request(path: string): Promise<unknown> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    let response: Response;
    try {
      response = await doFetch(`${baseUrl}${path}`, {
        signal: controller.signal,
        headers: { accept: 'application/json' },
      });
    } catch (cause) {
      throw new ApiError(0, {
        code: 'unavailable',
        message: cause instanceof Error ? cause.message : 'network request failed',
        request_id: '',
      });
    } finally {
      clearTimeout(timer);
    }

    const payload: unknown = await response.json().catch(() => null);

    if (!response.ok) {
      const parsed = apiErrorBodySchema.safeParse(payload);
      throw new ApiError(
        response.status,
        parsed.success
          ? parsed.data
          : {
              code: 'internal',
              message: `unexpected ${response.status} response`,
              request_id: response.headers.get('x-request-id') ?? '',
            },
      );
    }

    return payload;
  }

  return {
    async health(): Promise<HealthResponse> {
      return healthResponseSchema.parse(await request('/health'));
    },
  };
}
