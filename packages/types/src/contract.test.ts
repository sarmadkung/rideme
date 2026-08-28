import { describe, expect, it } from 'vitest';
import {
  CURRENCIES,
  ERROR_CODES,
  HEALTH_STATUSES,
  formatMoney,
  isCurrency,
  isErrorCode,
  isHealthStatus,
} from './index.js';
import type { Money } from './index.js';

describe('generated contract', () => {
  it('carries the error taxonomy the Go API serves', () => {
    // These values are generated from services/api/pkg/httpx. If Go changes
    // and `make contracts` was not run, `make contracts-check` fails first.
    expect(ERROR_CODES).toEqual([
      'not_found',
      'unauthorized',
      'forbidden',
      'conflict',
      'validation',
      'unavailable',
      'internal',
    ]);
  });

  it('carries the health statuses and currencies', () => {
    expect(HEALTH_STATUSES).toEqual(['healthy', 'degraded', 'unhealthy']);
    expect(CURRENCIES).toEqual(['PKR']);
  });

  it('narrows unknown values', () => {
    expect(isErrorCode('conflict')).toBe(true);
    expect(isErrorCode('teapot')).toBe(false);
    expect(isErrorCode(undefined)).toBe(false);
    expect(isHealthStatus('degraded')).toBe(true);
    expect(isHealthStatus('fine')).toBe(false);
    expect(isCurrency('PKR')).toBe(true);
    expect(isCurrency('USD')).toBe(false);
  });
});

describe('formatMoney', () => {
  const pkr = (amount_minor: number): Money => ({ amount_minor, currency: 'PKR' });

  it('renders minor units as major units', () => {
    expect(formatMoney(pkr(0))).toBe('PKR 0.00');
    expect(formatMoney(pkr(5))).toBe('PKR 0.05');
    expect(formatMoney(pkr(150))).toBe('PKR 1.50');
    expect(formatMoney(pkr(123456))).toBe('PKR 1234.56');
  });

  it('renders negative amounts with the sign outside the currency', () => {
    expect(formatMoney(pkr(-12345))).toBe('-PKR 123.45');
    expect(formatMoney(pkr(-5))).toBe('-PKR 0.05');
  });

  it('agrees with the Go formatter on the same values', () => {
    // The same cases assert in services/api/pkg/money/money_test.go. Display
    // is the one money operation that exists on both sides, so the two must
    // not drift.
    expect(formatMoney(pkr(0))).toBe('PKR 0.00');
    expect(formatMoney(pkr(5))).toBe('PKR 0.05');
    expect(formatMoney(pkr(150))).toBe('PKR 1.50');
    expect(formatMoney(pkr(-12345))).toBe('-PKR 123.45');
  });
});
