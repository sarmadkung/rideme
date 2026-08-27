import { describe, expect, it } from 'vitest';
import { apiErrorBodySchema, healthResponseSchema } from './index.js';

describe('apiErrorBodySchema', () => {
  it('accepts the envelope the Go API emits', () => {
    const parsed = apiErrorBodySchema.parse({
      code: 'not_found',
      message: 'resource not found',
      request_id: '01J000000000000000000000',
    });
    expect(parsed.code).toBe('not_found');
  });

  it('rejects an undocumented code', () => {
    expect(() =>
      apiErrorBodySchema.parse({ code: 'teapot', message: 'x', request_id: 'y' }),
    ).toThrow();
  });
});

describe('healthResponseSchema', () => {
  it('accepts a degraded response with a failing dependency', () => {
    const parsed = healthResponseSchema.parse({
      status: 'unhealthy',
      service: 'api',
      version: 'dev',
      checked_at: '2026-01-01T00:00:00Z',
      dependencies: [
        { name: 'postgres', status: 'unhealthy', latency_ms: 12, error: 'connection refused' },
        { name: 'redis', status: 'healthy', latency_ms: 1 },
      ],
    });
    expect(parsed.dependencies).toHaveLength(2);
  });
});
