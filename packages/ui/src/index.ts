/**
 * Shared design primitives (document 23).
 *
 * Tokens and helpers only. The component library is deliberately absent —
 * building one before there is a screen to put it in produces components nobody
 * uses. Web and mobile implementations diverge where they must.
 */

export const tokens = {
  color: {
    background: '#0b0d10',
    surface: '#14181d',
    border: '#232a32',
    text: '#e8edf2',
    textMuted: '#8b98a5',
    accent: '#3b82f6',
    success: '#22c55e',
    warning: '#f59e0b',
    danger: '#ef4444',
  },
  space: { xs: 4, sm: 8, md: 16, lg: 24, xl: 40 },
  radius: { sm: 4, md: 8, lg: 16 },
  fontSize: { sm: 13, md: 15, lg: 20, xl: 28 },
} as const;

export type Tokens = typeof tokens;

/** Joins class names, dropping falsy entries. */
export function cn(...values: Array<string | false | null | undefined>): string {
  return values.filter((value): value is string => Boolean(value)).join(' ');
}

/**
 * Minor units per currency, mirroring the server's table (ADR-008).
 *
 * Money crosses the wire as an integer count of the smallest unit, and it must
 * stay that way until the moment it is displayed. Formatting is the only place
 * a division by 100 is correct.
 */
const MINOR_UNITS: Record<string, number> = { PKR: 100 };

export interface MoneyLike {
  amount_minor: number;
  currency: string;
}

/**
 * Formats an amount for display.
 *
 * Negative amounts keep their sign rather than being shown in parentheses: a
 * discount line reading "-PKR 50.00" is unambiguous, and a customer should not
 * have to know an accounting convention to read their own fare.
 */
export function formatMoney(money: MoneyLike): string {
  const divisor = MINOR_UNITS[money.currency] ?? 100;
  const negative = money.amount_minor < 0;
  const absolute = Math.abs(money.amount_minor);
  const whole = Math.trunc(absolute / divisor);
  const fraction = absolute % divisor;
  const digits = String(divisor - 1).length;
  const grouped = whole.toLocaleString('en-US');
  const formatted = `${money.currency} ${grouped}.${String(fraction).padStart(digits, '0')}`;
  return negative ? `-${formatted}` : formatted;
}

/** Human-readable labels for the fare components the server sends. */
const COMPONENT_LABELS: Record<string, string> = {
  base: 'Base fare',
  distance: 'Distance',
  time: 'Time',
  waiting: 'Waiting',
  loading: 'Loading',
  weight: 'Weight',
  service_fee: 'Service fee',
  demand: 'Busy area',
  discount: 'Discount',
  tax: 'Tax',
  minimum_fare_top_up: 'Minimum fare',
};

/**
 * Names a fare component for a customer.
 *
 * "demand" becomes "Busy area" rather than "Surge": it is what the multiplier
 * actually means, and it is the wording a customer can act on by waiting.
 * An unrecognised component falls back to its raw name, so a new server-side
 * line shows up as something rather than silently vanishing from a total the
 * customer is still charged.
 */
export function fareComponentLabel(component: string): string {
  return COMPONENT_LABELS[component] ?? component;
}
