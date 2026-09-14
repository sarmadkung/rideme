import { createApiClient, type ApiClient } from '@platform/api-client';
import { secureTokenStorage } from '@platform/mobile';
import * as Application from 'expo-application';
import { Platform } from 'react-native';
import { loadEnv } from '../env';

/**
 * The app's single API client.
 *
 * One instance, not one per screen: the client holds the session and coalesces
 * refreshes, and a second instance would refresh independently — presenting a
 * rotated token the server has already retired.
 */
let client: ApiClient | null = null;

export interface ClientOptions {
  onSessionExpired: () => void;
}

export function getApiClient(options: ClientOptions): ApiClient {
  if (client) return client;

  client = createApiClient({
    baseUrl: loadEnv(process.env as Record<string, string | undefined>).apiBaseUrl,
    storage: secureTokenStorage(),
    device: {
      // getAndroidId exists on iOS but throws when called, so the optional
      // call is no guard at all — the platform check is. iOS has no equivalent
      // synchronous id (identifierForVendor is async), so it sends none.
      id: Platform.OS === 'android' ? (Application.getAndroidId() ?? undefined) : undefined,
      platform: Platform.OS,
      os: String(Platform.Version),
      appVersion: Application.nativeApplicationVersion ?? undefined,
    },
    onSessionExpired: options.onSessionExpired,
  });
  return client;
}

/** Used by tests to start from a clean client. */
export function resetApiClient(): void {
  client = null;
}
