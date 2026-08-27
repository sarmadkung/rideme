/**
 * Shared, non-secret configuration conventions (document 23).
 *
 * This package never reads a secret. Client bundles may only ever contain
 * variables carrying a public prefix (`VITE_`, `EXPO_PUBLIC_`); everything else
 * belongs to the Go API's server-side configuration.
 */

export const APP_ENVS = ['development', 'test', 'staging', 'production'] as const;

export type AppEnv = (typeof APP_ENVS)[number];

export function isAppEnv(value: unknown): value is AppEnv {
  return typeof value === 'string' && (APP_ENVS as readonly string[]).includes(value);
}

export class ConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ConfigError';
  }
}

/** Public prefixes. A variable without one must never reach a client bundle. */
export const PUBLIC_ENV_PREFIXES = ['VITE_', 'EXPO_PUBLIC_'] as const;

export function isPublicEnvKey(key: string): boolean {
  return PUBLIC_ENV_PREFIXES.some((prefix) => key.startsWith(prefix));
}

/** Reads a required variable, failing loudly at startup rather than at use. */
export function requireEnv(source: Record<string, string | undefined>, key: string): string {
  const value = source[key];
  if (value === undefined || value.trim() === '') {
    throw new ConfigError(`Missing required environment variable: ${key}`);
  }
  return value;
}

export function requireAppEnv(source: Record<string, string | undefined>, key: string): AppEnv {
  const value = requireEnv(source, key);
  if (!isAppEnv(value)) {
    throw new ConfigError(`${key} must be one of ${APP_ENVS.join(', ')}, got "${value}"`);
  }
  return value;
}

/** Rejects a secret that has been given a public prefix by mistake. */
export function assertNoSecretsInPublicEnv(source: Record<string, string | undefined>): void {
  const suspicious = /(SECRET|PASSWORD|PRIVATE_KEY|TOKEN|CREDENTIAL)/i;
  const leaked = Object.keys(source).filter((key) => isPublicEnvKey(key) && suspicious.test(key));
  if (leaked.length > 0) {
    throw new ConfigError(
      `Public environment variables must not hold secrets: ${leaked.join(', ')}`,
    );
  }
}
