import { createApiClient, memoryTokenStorage, type ApiClient } from '@platform/api-client';
import { env } from '../env';

/**
 * The dashboard's API client.
 *
 * Refresh credentials are held in memory rather than in localStorage. An
 * operator console is the highest-privilege surface on the platform, and a
 * long-lived token in localStorage is readable by any script that reaches the
 * page. In-memory means closing the tab ends the session, which for an
 * operations console is the right trade.
 */
let client: ApiClient | null = null;

export function getApiClient(onSessionExpired: () => void): ApiClient {
  if (client) return client;
  client = createApiClient({
    baseUrl: env.apiBaseUrl,
    storage: memoryTokenStorage(),
    onSessionExpired,
  });
  return client;
}

export function resetApiClient(): void {
  client = null;
}
