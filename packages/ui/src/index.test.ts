import { describe, expect, it } from 'vitest';
import { cn, fareComponentLabel, formatMoney, tokens } from './index.js';

describe('cn', () => {
  it('drops falsy values', () => {
    expect(cn('a', false, undefined, 'b', null, '')).toBe('a b');
  });
});

describe('tokens', () => {
  it('exposes a complete colour scale', () => {
    expect(Object.keys(tokens.color)).toContain('danger');
    expect(tokens.space.md).toBe(16);
  });
});

describe('formatMoney', () => {
  it('renders whole and fractional units from an integer', () => {
    expect(formatMoney({ amount_minor: 12345, currency: 'PKR' })).toBe('PKR 123.45');
    expect(formatMoney({ amount_minor: 10000, currency: 'PKR' })).toBe('PKR 100.00');
  });

  it('pads the fraction so amounts line up', () => {
    // 5 minor units is five paisa, not fifty. Without padding this reads as
    // ten times the actual amount.
    expect(formatMoney({ amount_minor: 105, currency: 'PKR' })).toBe('PKR 1.05');
  });

  it('groups thousands', () => {
    expect(formatMoney({ amount_minor: 123456789, currency: 'PKR' })).toBe('PKR 1,234,567.89');
  });

  it('keeps the sign on a negative amount', () => {
    // A discount line. Parentheses would make a customer read an accounting
    // convention to understand their own fare.
    expect(formatMoney({ amount_minor: -5000, currency: 'PKR' })).toBe('-PKR 50.00');
  });

  it('renders zero without a special case', () => {
    expect(formatMoney({ amount_minor: 0, currency: 'PKR' })).toBe('PKR 0.00');
  });
});

describe('fareComponentLabel', () => {
  it('names the demand line for what a customer can act on', () => {
    expect(fareComponentLabel('demand')).toBe('Busy area');
  });

  it('falls back to the raw name for an unknown component', () => {
    // A new server-side line must show up as something. Dropping it would hide
    // part of a total the customer is still charged.
    expect(fareComponentLabel('congestion_levy')).toBe('congestion_levy');
  });
});
