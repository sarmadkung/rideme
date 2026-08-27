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
