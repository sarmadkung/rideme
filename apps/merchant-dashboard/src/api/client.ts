import { createApiClient, memoryTokenStorage, type ApiClient } from '@platform/api-client';
import { env } from '../env';

/**
 * The dashboard's API client.
 *
 * Refresh credentials are held in memory, as they are in the admin console. A
 * shop's dashboard runs on a counter terminal that staff share and rarely lock,
 * and a token in localStorage outlives the shift, the person and the tab. In
 * memory means closing the browser ends the session, which is the right trade
 * for a screen that can reject a customer's order.
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
