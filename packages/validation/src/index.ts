/**
 * Runtime validation for the wire contract (document 23).
 *
 * Every schema is generated from the same Go types as `@platform/types`
 * (ADR-007), so a schema cannot describe a shape the types do not, or a shape
 * the server does not serve. Do not hand-write a schema for an API payload —
 * change the Go type and run `make contracts`.
 *
 * The division of labour: `@platform/types` is compile-time shape,
 * `@platform/validation` is the runtime check at the boundary where untyped
 * data arrives. Both come from one source.
 */
export * from './generated.js';

import type { apiErrorBodySchema, healthResponseSchema, moneySchema } from './generated.js';
import type { z } from 'zod';

export type ApiErrorBodyInput = z.infer<typeof apiErrorBodySchema>;
export type HealthResponseInput = z.infer<typeof healthResponseSchema>;
export type MoneyInput = z.infer<typeof moneySchema>;
