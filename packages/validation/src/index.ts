/**
 * Zod schemas shared across clients (document 23).
 *
 * Phase 1 covers only the transport envelopes the Go API actually serves today:
 * the error envelope and the health response. Domain schemas belong to Phase 5.
 *
 * These schemas are the third hand-maintained copy of one contract — Go structs
 * in `services/api/pkg/httpx`, TypeScript types in `@platform/types`, Zod here.
 * Choosing a single source of truth is B-2 in docs/BLOCKED_TASKS.md and is due
 * before any domain payload is added.
 */
import { z } from 'zod';
import { ERROR_CODES } from '@platform/types';

export const errorCodeSchema = z.enum(ERROR_CODES);

export const apiErrorBodySchema = z.object({
  code: errorCodeSchema,
  message: z.string(),
  request_id: z.string(),
  details: z.record(z.string(), z.string()).optional(),
});

export const healthStatusSchema = z.enum(['healthy', 'degraded', 'unhealthy']);

export const dependencyHealthSchema = z.object({
  name: z.string(),
  status: healthStatusSchema,
  latency_ms: z.number(),
  error: z.string().optional(),
});

export const healthResponseSchema = z.object({
  status: healthStatusSchema,
  service: z.string(),
  version: z.string(),
  checked_at: z.string(),
  dependencies: z.array(dependencyHealthSchema),
});

export type ApiErrorBodyInput = z.infer<typeof apiErrorBodySchema>;
export type HealthResponseInput = z.infer<typeof healthResponseSchema>;
