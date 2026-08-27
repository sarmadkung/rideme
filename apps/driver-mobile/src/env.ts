import {
  assertNoSecretsInPublicEnv,
  requireAppEnv,
  requireEnv,
  type AppEnv,
} from '@platform/config';

export interface MobileEnv {
  appEnv: AppEnv;
  apiBaseUrl: string;
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
  };
}

export function loadEnvOrNull(source: Record<string, string | undefined>): MobileEnv | null {
  try {
    return loadEnv(source);
  } catch {
    return null;
  }
}
