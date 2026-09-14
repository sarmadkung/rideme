import {
  assertNoSecretsInPublicEnv,
  requireAppEnv,
  requireEnv,
  type AppEnv,
} from '@platform/config';

export interface MerchantEnv {
  appEnv: AppEnv;
  apiBaseUrl: string;
}

/**
 * Validated once, at module load, so a misconfigured build fails immediately
 * instead of at the first request (document 25 applies the same rule server-side).
 */
export function loadEnv(source: Record<string, string | undefined>): MerchantEnv {
  assertNoSecretsInPublicEnv(source);
  return {
    appEnv: requireAppEnv(source, 'VITE_APP_ENV'),
    apiBaseUrl: requireEnv(source, 'VITE_API_BASE_URL'),
  };
}

export const env: MerchantEnv = loadEnv(import.meta.env as Record<string, string | undefined>);
