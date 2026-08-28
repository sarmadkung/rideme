import { describe, expect, it } from 'vitest';
import {
  analyticsEventSchema,
  apiErrorBodySchema,
  healthResponseSchema,
  moneySchema,
  pageInfoSchema,
} from './index.js';

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

  it('rejects a timestamp that is not RFC 3339', () => {
    expect(() =>
      healthResponseSchema.parse({
        status: 'healthy',
        service: 'api',
        version: 'dev',
        checked_at: '2026-01-01 00:00:00',
        dependencies: [],
      }),
    ).toThrow();
  });
});

describe('moneySchema', () => {
  it('accepts an integer minor-unit amount', () => {
    expect(moneySchema.parse({ amount_minor: -12345, currency: 'PKR' }).amount_minor).toBe(-12345);
  });

  it('rejects a fractional amount', () => {
    // BD-07: money is an integer count of minor units. A client that sends
    // 10.5 paisa is sending a value the server cannot represent.
    expect(() => moneySchema.parse({ amount_minor: 10.5, currency: 'PKR' })).toThrow();
  });

  it('rejects an unknown currency', () => {
    expect(() => moneySchema.parse({ amount_minor: 100, currency: 'USD' })).toThrow();
  });

  it('rejects an amount past the safe integer bound', () => {
    // Beyond MAX_SAFE_INTEGER this client would silently disagree with the
    // server about the value it holds.
    expect(() => moneySchema.parse({ amount_minor: 9007199254740992, currency: 'PKR' })).toThrow();
    expect(moneySchema.parse({ amount_minor: 9007199254740991, currency: 'PKR' })).toBeTruthy();
  });
});

describe('analyticsEventSchema', () => {
  const valid = {
    event_id: '01J000000000000000000000',
    event_name: 'ride.booked',
    actor_id: 'user_1',
    timestamp: '2026-08-28T10:30:00Z',
    source: 'customer-mobile',
  };

  it('accepts the documented envelope', () => {
    expect(analyticsEventSchema.parse(valid).event_name).toBe('ride.booked');
  });

  it('enforces the same event-name shape the Go validator does', () => {
    for (const event_name of ['ride', 'Ride.Booked', 'ride..booked', 'ride-booked']) {
      expect(() => analyticsEventSchema.parse({ ...valid, event_name })).toThrow();
    }
  });

  it('rejects a timestamp that is not RFC 3339', () => {
    expect(() => analyticsEventSchema.parse({ ...valid, timestamp: 'yesterday' })).toThrow();
  });
});

describe('pageInfoSchema', () => {
  it('treats next_cursor as optional so the last page can omit it', () => {
    expect(pageInfoSchema.parse({ limit: 25 }).next_cursor).toBeUndefined();
    expect(pageInfoSchema.parse({ limit: 25, next_cursor: 'abc' }).next_cursor).toBe('abc');
  });
});
