/**
 * The platform's wire contract, as TypeScript.
 *
 * Every type here is generated from the Go types that serve it
 * (`services/api/cmd/contractgen`, ADR-007). Do not hand-write an interface for
 * anything the API returns — change the Go type and run `make contracts`.
 * Hand-written declarations are what B-2 removed.
 *
 * This file adds only what generation cannot: runtime helpers over generated
 * values, and display formatting.
 */
export * from './generated.js';

import { CURRENCIES, ERROR_CODES, HEALTH_STATUSES } from './generated.js';
import type { Currency, ErrorCode, HealthStatus, Money } from './generated.js';

export function isErrorCode(value: unknown): value is ErrorCode {
  return typeof value === 'string' && (ERROR_CODES as readonly string[]).includes(value);
}

export function isHealthStatus(value: unknown): value is HealthStatus {
  return typeof value === 'string' && (HEALTH_STATUSES as readonly string[]).includes(value);
}

export function isCurrency(value: unknown): value is Currency {
  return typeof value === 'string' && (CURRENCIES as readonly string[]).includes(value);
}

/** Minor units per major unit, by currency. PKR has 100 paisa to the rupee. */
const MINOR_UNITS: Record<Currency, number> = { PKR: 100 };

/**
 * Renders an amount for display.
 *
 * Formatting is the *only* money operation clients perform. There is
 * deliberately no client-side arithmetic: the backend is authoritative for
 * every amount, and a fare added up in a browser is a second implementation of
 * a rule that already exists on the server (BD-07, CAP-1). Clients display
 * what they are given.
 */
export function formatMoney(amount: Money): string {
  const scale = MINOR_UNITS[amount.currency];
  const negative = amount.amount_minor < 0;
  const magnitude = Math.abs(amount.amount_minor);
  const major = Math.trunc(magnitude / scale);
  const minor = magnitude % scale;
  const digits = String(scale).length - 1;
  return `${negative ? '-' : ''}${amount.currency} ${major}.${String(minor).padStart(digits, '0')}`;
}
