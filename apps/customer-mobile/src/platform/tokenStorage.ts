import * as SecureStore from 'expo-secure-store';
import type { TokenStorage } from '@platform/api-client';

/**
 * Refresh-token storage backed by the platform keystore.
 *
 * Document 28 is explicit: refresh credentials must use iOS Keychain or
 * Android Keystore-backed storage, and "Do not store sensitive tokens in plain
 * AsyncStorage." A refresh token is long-lived and, until it is rotated,
 * enough to impersonate the user — AsyncStorage is a plaintext file readable
 * by anything with filesystem access on a rooted or jailbroken device.
 *
 * `expo-secure-store` maps to Keychain on iOS and to the Keystore-backed
 * EncryptedSharedPreferences on Android, which is exactly what the document
 * asks for.
 */
const REFRESH_TOKEN_KEY = 'rideme.refresh_token';

export function secureTokenStorage(): TokenStorage {
  return {
    async get() {
      try {
        return await SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
      } catch {
        // A keystore that cannot be read means no session, not a crash on
        // launch. The user logs in again.
        return null;
      }
    },

    async set(token: string) {
      await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, token, {
        // The token is only needed while someone is using the app, so it does
        // not need to survive a locked device.
        keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
      });
    },

    async clear() {
      try {
        await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
      } catch {
        // Already gone is the desired state.
      }
    },
  };
}
