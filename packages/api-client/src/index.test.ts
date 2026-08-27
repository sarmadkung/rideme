import { describe, expect, it, vi } from 'vitest';
import { ApiError, createApiClient } from './index.js';

const jsonResponse = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });

const healthy = {
  status: 'healthy',
  service: 'api',
  version: 'dev',
  checked_at: '2026-01-01T00:00:00Z',
  dependencies: [{ name: 'postgres', status: 'healthy', latency_ms: 3 }],
};

describe('createApiClient', () => {
  it('parses a healthy response and strips trailing slashes from the base URL', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, healthy));
    const client = createApiClient({ baseUrl: 'http://localhost:8080/', fetch: fetchMock });

    await expect(client.health()).resolves.toMatchObject({ status: 'healthy' });
    expect(fetchMock).toHaveBeenCalledWith('http://localhost:8080/health', expect.anything());
  });

  it('maps the API error envelope onto ApiError', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(503, {
        code: 'unavailable',
        message: 'postgres unreachable',
        request_id: 'req-1',
      }),
    );
    const client = createApiClient({ baseUrl: 'http://localhost:8080', fetch: fetchMock });

    await expect(client.health()).rejects.toMatchObject({
      name: 'ApiError',
      code: 'unavailable',
      status: 503,
      requestId: 'req-1',
    });
  });

  it('reports a transport failure as unavailable rather than leaking the cause', async () => {
    const fetchMock = vi.fn(async () => {
      throw new Error('connect ECONNREFUSED');
    });
    const client = createApiClient({ baseUrl: 'http://localhost:8080', fetch: fetchMock });

    const error = await client.health().catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe('unavailable');
    expect((error as ApiError).status).toBe(0);
  });
});
