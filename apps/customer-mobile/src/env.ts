import {
  assertNoSecretsInPublicEnv,
  requireAppEnv,
  requireEnv,
  type AppEnv,
} from '@platform/config';

export interface MobileEnv {
  appEnv: AppEnv;
  apiBaseUrl: string;
  /**
   * The market this build serves, used to select a pricing tariff.
   *
   * Optional, and deliberately not defaulted. Tariffs are per city and the
   * platform ships none — inventing a city here would ask the server for a
   * fare in a market nobody configured. Left unset, quoting fails with the
   * server's own "not available here yet", which is the truth.
   */
  city?: string;
}

/**
 * EXPO_PUBLIC_* variables are inlined into the shipped bundle. Anything read
 * here is public by definition — a secret must never appear behind this prefix,
 * which assertNoSecretsInPublicEnv enforces rather than merely documents.
 */
export function loadEnv(source: Record<string, string | undefined>): MobileEnv {
  assertNoSecretsInPublicEnv(source);
  return {
    appEnv: requireAppEnv(source, 'EXPO_PUBLIC_APP_ENV'),
    apiBaseUrl: requireEnv(source, 'EXPO_PUBLIC_API_BASE_URL'),
    city: source.EXPO_PUBLIC_CITY,
  };
}

export function loadEnvOrNull(source: Record<string, string | undefined>): MobileEnv | null {
  try {
    return loadEnv(source);
  } catch {
    return null;
  }
}
