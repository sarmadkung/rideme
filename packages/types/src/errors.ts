/**
 * Mirror of the Go backend's typed error taxonomy (`services/api/pkg/httpx`,
 * document 25). This is a transport contract, not a domain model — domain types
 * arrive in Phase 5.
 *
 * These two lists are hand-kept in sync with Go today. Choosing a generation
 * strategy is tracked as B-2 in docs/BLOCKED_TASKS.md and is due before the
 * first real endpoint.
 */
export const ERROR_CODES = [
  'not_found',
  'unauthorized',
  'forbidden',
  'conflict',
  'validation',
  'unavailable',
  'internal',
] as const;

export type ErrorCode = (typeof ERROR_CODES)[number];

export function isErrorCode(value: unknown): value is ErrorCode {
  return typeof value === 'string' && (ERROR_CODES as readonly string[]).includes(value);
}

/** Envelope returned by the API for every non-2xx response. */
export interface ApiErrorBody {
  code: ErrorCode;
  message: string;
  request_id: string;
  details?: Record<string, string> | undefined;
}
