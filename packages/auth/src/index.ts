/**
 * Authentication abstractions (document 23).
 *
 * Phase 1 defines the seam only. Registration, OTP, sessions, refresh and RBAC
 * are Phase 4 — nothing here talks to the API or decodes a token.
 */

export interface AuthTokens {
  readonly accessToken: string;
  readonly refreshToken: string;
  /** Unix epoch milliseconds. */
  readonly expiresAt: number;
}

/**
 * Where tokens live. Web uses one implementation, mobile another backed by the
 * platform keystore (document 17: secure storage is a native concern).
 */
export interface TokenStorage {
  read(): Promise<AuthTokens | null>;
  write(tokens: AuthTokens): Promise<void>;
  clear(): Promise<void>;
}

/** Test and development double. Never use this to hold a real token. */
export class InMemoryTokenStorage implements TokenStorage {
  #tokens: AuthTokens | null = null;

  async read(): Promise<AuthTokens | null> {
    return this.#tokens;
  }

  async write(tokens: AuthTokens): Promise<void> {
    this.#tokens = tokens;
  }

  async clear(): Promise<void> {
    this.#tokens = null;
  }
}

export function isExpired(tokens: AuthTokens, now: number, skewMs = 30_000): boolean {
  return tokens.expiresAt - skewMs <= now;
}
