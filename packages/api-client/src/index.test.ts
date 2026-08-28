import { describe, expect, it, vi } from 'vitest';
import { ApiError, createApiClient, memoryTokenStorage } from './index.js';

type Route = { status: number; body: unknown; headers?: Record<string, string> };

/** A fetch stand-in that records calls and replies from a routing table. */
function stubFetch(routes: Record<string, Route | Route[]>) {
  const calls: { url: string; method: string; headers: Record<string, string>; body?: unknown }[] =
    [];
  const remaining = new Map<string, Route[]>();
  for (const [key, value] of Object.entries(routes)) {
    remaining.set(key, Array.isArray(value) ? [...value] : [value]);
  }

  const fetchImpl = vi.fn(async (input: URL | RequestInfo, init?: RequestInit) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
    const method = init?.method ?? 'GET';
    const key = `${method} ${new URL(url).pathname}`;
    calls.push({
      url,
      method,
      headers: (init?.headers ?? {}) as Record<string, string>,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    });

    const queue = remaining.get(key);
    if (!queue || queue.length === 0) {
      return new Response(JSON.stringify({ code: 'not_found', message: key, request_id: 'r' }), {
        status: 404,
        headers: { 'content-type': 'application/json' },
      });
    }
    const route = queue.length > 1 ? queue.shift()! : queue[0]!;
    return new Response(route.status === 204 ? null : JSON.stringify(route.body), {
      status: route.status,
      headers: { 'content-type': 'application/json', ...(route.headers ?? {}) },
    });
  });

  return { fetchImpl, calls };
}

const tokens = {
  access_token: 'access-1',
  refresh_token: 'refresh-1',
  expires_at: '2026-08-28T12:05:00Z',
  user: {
    id: 'u1',
    phone: '+923001234567',
    status: 'ACTIVE',
    roles: ['CUSTOMER'],
    created_at: '2026-08-28T12:00:00Z',
  },
};

const job = {
  id: 'job-1',
  type: 'RIDE',
  status: 'REQUESTED',
  stops: [{ id: 's1', sequence: 0, type: 'PICKUP', latitude: 31.5204, longitude: 74.3587 }],
  created_at: '2026-08-28T12:00:00Z',
};

describe('authentication', () => {
  it('stores the refresh token and authenticates later calls', async () => {
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs': { status: 200, body: { items: [job], page: { limit: 25 } } },
    });
    const storage = memoryTokenStorage();
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl, storage });

    await client.verifyOtp('03001234567', '123456');
    expect(await storage.get()).toBe('refresh-1');
    expect(client.isAuthenticated()).toBe(true);

    await client.listJobs();
    const jobsCall = calls.find((c) => c.url.includes('/jobs'));
    expect(jobsCall?.headers['authorization']).toBe('Bearer access-1');
  });

  it('never sends the refresh token as a bearer credential', async () => {
    // The refresh token is long-lived; putting it in an Authorization header
    // would expose it to every proxy and log on the path.
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });
    await client.verifyOtp('03001234567', '123456');

    for (const call of calls) {
      // Anonymous calls carry no Authorization header at all, which is also
      // correct — the assertion is that the refresh token never appears in one.
      expect(call.headers['authorization'] ?? '').not.toContain('refresh-1');
    }
  });

  it('clears local state on logout even if the call fails', async () => {
    // A user who taps log out must be logged out of this device regardless of
    // what the network does.
    const { fetchImpl } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'POST /api/v1/auth/logout': {
        status: 503,
        body: { code: 'unavailable', message: 'down', request_id: 'r' },
      },
    });
    const storage = memoryTokenStorage();
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl, storage });

    await client.verifyOtp('03001234567', '123456');
    await expect(client.logout()).rejects.toBeInstanceOf(ApiError);

    expect(await storage.get()).toBeNull();
    expect(client.isAuthenticated()).toBe(false);
  });
});

describe('token refresh', () => {
  it('refreshes once and retries the original request', async () => {
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs': [
        { status: 401, body: { code: 'unauthorized', message: 'expired', request_id: 'r' } },
        { status: 200, body: { items: [job], page: { limit: 25 } } },
      ],
      'POST /api/v1/auth/refresh': {
        status: 200,
        body: { ...tokens, access_token: 'access-2', refresh_token: 'refresh-2' },
      },
    });
    const storage = memoryTokenStorage();
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl, storage });

    await client.verifyOtp('03001234567', '123456');
    const result = await client.listJobs();

    expect(result.items).toHaveLength(1);
    // The rotated token was stored, or the next refresh would present a
    // retired one.
    expect(await storage.get()).toBe('refresh-2');
    expect(calls.filter((c) => c.url.includes('/auth/refresh'))).toHaveLength(1);
  });

  it('coalesces concurrent refreshes into one', async () => {
    // This is the important one. Refresh tokens rotate, so five parallel
    // refreshes would present a token the server has already retired — which
    // it correctly reads as theft and answers by ending every session.
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs': [
        { status: 401, body: { code: 'unauthorized', message: 'expired', request_id: 'r' } },
        { status: 401, body: { code: 'unauthorized', message: 'expired', request_id: 'r' } },
        { status: 401, body: { code: 'unauthorized', message: 'expired', request_id: 'r' } },
        { status: 401, body: { code: 'unauthorized', message: 'expired', request_id: 'r' } },
        { status: 200, body: { items: [], page: { limit: 25 } } },
      ],
      'POST /api/v1/auth/refresh': {
        status: 200,
        body: { ...tokens, access_token: 'access-2', refresh_token: 'refresh-2' },
      },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });
    await client.verifyOtp('03001234567', '123456');

    await Promise.all([client.listJobs(), client.listJobs(), client.listJobs(), client.listJobs()]);

    expect(calls.filter((c) => c.url.includes('/auth/refresh'))).toHaveLength(1);
  });

  it('reports session expiry when the refresh token is gone', async () => {
    const onSessionExpired = vi.fn();
    const { fetchImpl } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs': {
        status: 401,
        body: { code: 'unauthorized', message: 'expired', request_id: 'r' },
      },
      'POST /api/v1/auth/refresh': {
        status: 401,
        body: { code: 'unauthorized', message: 'session revoked', request_id: 'r' },
      },
    });
    const storage = memoryTokenStorage();
    const client = createApiClient({
      baseUrl: 'https://api.test',
      fetch: fetchImpl,
      storage,
      onSessionExpired,
    });

    await client.verifyOtp('03001234567', '123456');
    await expect(client.listJobs()).rejects.toBeInstanceOf(ApiError);

    expect(onSessionExpired).toHaveBeenCalledOnce();
    // A dead refresh token is cleared, so the next launch shows the login
    // screen rather than retrying a token the server has revoked.
    expect(await storage.get()).toBeNull();
    expect(client.isAuthenticated()).toBe(false);
  });
});

describe('requests', () => {
  it('sends the idempotency key on job creation', async () => {
    // Document 14 requires it, and a retry of the same user action must reuse
    // the same key or the customer gets two rides.
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'POST /api/v1/jobs': { status: 201, body: job },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });
    await client.verifyOtp('03001234567', '123456');

    await client.createJob(
      { quoteId: 'q1', jobType: 'RIDE', stops: [{ latitude: 31.5, longitude: 74.3 }] },
      'idem-key-1',
    );

    const create = calls.find((c) => c.method === 'POST' && c.url.endsWith('/jobs'));
    expect(create?.headers['Idempotency-Key']).toBe('idem-key-1');
  });

  it('sends device signals when configured', async () => {
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/request': {
        status: 202,
        body: { expires_at: '2026-08-28T12:05:00Z' },
      },
    });
    const client = createApiClient({
      baseUrl: 'https://api.test',
      fetch: fetchImpl,
      device: { id: 'device-1', platform: 'ios', os: '18.0', appVersion: '1.0.0' },
    });

    await client.requestOtp('03001234567');
    expect(calls[0]?.headers['X-Device-Id']).toBe('device-1');
    expect(calls[0]?.headers['X-Platform']).toBe('ios');
  });

  it('surfaces a network failure in the platform error shape', async () => {
    // Callers handle one kind of error rather than two.
    const fetchImpl: typeof globalThis.fetch = async () => {
      throw new TypeError('Network request failed');
    };
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });

    await expect(client.requestOtp('03001234567')).rejects.toMatchObject({
      name: 'ApiError',
      code: 'unavailable',
      status: 0,
    });
  });

  it('classifies errors so callers know whether to retry or re-login', async () => {
    const { fetchImpl } = stubFetch({
      'POST /api/v1/auth/otp/request': {
        status: 429,
        body: { code: 'rate_limited', message: 'too many attempts', request_id: 'r' },
      },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });

    try {
      await client.requestOtp('03001234567');
      expect.unreachable('should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      const apiError = error as ApiError;
      expect(apiError.retryable).toBe(true);
      expect(apiError.requiresLogin).toBe(false);
    }
  });

  it('passes the cursor through and reports the next one', async () => {
    const { fetchImpl, calls } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs': {
        status: 200,
        body: { items: [job], page: { limit: 1, next_cursor: '2026-08-28T11:00:00Z' } },
      },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });
    await client.verifyOtp('03001234567', '123456');

    const page = await client.listJobs({ limit: 1, cursor: '2026-08-28T12:00:00Z' });
    expect(page.nextCursor).toBe('2026-08-28T11:00:00Z');
    const listCall = calls.find((c) => c.method === 'GET' && c.url.includes('/jobs'));
    expect(listCall?.url).toContain('cursor=2026-08-28T12%3A00%3A00Z');
  });

  it('rejects a response that does not match the contract', async () => {
    // The generated schema is the boundary check: a server that changes shape
    // without regenerating is caught here rather than crashing a screen.
    const { fetchImpl } = stubFetch({
      'POST /api/v1/auth/otp/verify': { status: 200, body: tokens },
      'GET /api/v1/jobs/job-1': { status: 200, body: { id: 'job-1', unexpected: true } },
    });
    const client = createApiClient({ baseUrl: 'https://api.test', fetch: fetchImpl });
    await client.verifyOtp('03001234567', '123456');

    await expect(client.getJob('job-1')).rejects.toThrow();
  });
});
