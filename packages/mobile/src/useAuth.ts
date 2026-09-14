import { useCallback, useMemo, useState } from 'react';
import { ApiError, type ApiClient } from '@platform/api-client';

/**
 * The phone-OTP login flow from document 28, as app state.
 *
 * It lives in shared TypeScript rather than in a screen, and there is one copy
 * for both platforms: document 48 is explicit that business logic must not be
 * duplicated per platform, and an auth flow that differs between iOS and
 * Android differs in exactly the ways nobody tests.
 *
 * This file has no React Native dependency, unlike tokenStorage.ts (which
 * wraps expo-secure-store) — the package's other export. A web app that needs
 * only this flow imports the `@platform/mobile/useAuth` subpath rather than
 * the package root, so its bundler never has to resolve a native-only module
 * it will never call.
 */
export type AuthStage = 'phone' | 'code' | 'authenticated';

export interface AuthState {
  stage: AuthStage;
  phone: string;
  pending: boolean;
  error: string | null;
  /** When the current code expires, so the UI can offer a resend. */
  codeExpiresAt: Date | null;
  /**
   * The signed-in user's roles, from the server's verify response. Empty
   * until authenticated. Most screens never look at this — it exists for the
   * one surface that gates on a role (the admin dashboard), rather than
   * having that surface re-derive session state on its own.
   */
  roles: string[];
}

export interface AuthActions {
  requestCode(phone: string): Promise<void>;
  submitCode(code: string): Promise<boolean>;
  restart(): void;
}

export interface UseAuthOptions {
  /**
   * Local-testing only. When set, requestCode submits this code itself
   * instead of moving to the 'code' stage — the server is the real guard
   * (AUTH_OTP_BYPASS, refused outside development), this only skips a screen
   * that would otherwise pass anyway.
   */
  devAutoCode?: string;
}

/**
 * The code a bypass-enabled server accepts unconditionally. Any value works
 * server-side once AUTH_OTP_BYPASS is on; this exists so both apps send the
 * same one rather than each inventing a placeholder.
 */
export const DEV_OTP_BYPASS_CODE = '000000';

const INITIAL: AuthState = {
  stage: 'phone',
  phone: '',
  pending: false,
  error: null,
  codeExpiresAt: null,
  roles: [],
};

export function useAuth(client: ApiClient, options: UseAuthOptions = {}): AuthState & AuthActions {
  const [state, setState] = useState<AuthState>(INITIAL);
  const { devAutoCode } = options;

  const verify = useCallback(
    async (phone: string, code: string) => {
      setState((s) => ({ ...s, phone, pending: true, error: null }));
      try {
        const session = await client.verifyOtp(phone, code);
        setState((s) => ({
          ...s,
          stage: 'authenticated',
          pending: false,
          error: null,
          roles: session.roles,
        }));
        return true;
      } catch (error) {
        setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
        return false;
      }
    },
    [client],
  );

  const requestCode = useCallback(
    async (phone: string) => {
      setState((s) => ({ ...s, pending: true, error: null }));
      try {
        const { expiresAt } = await client.requestOtp(phone);
        if (devAutoCode !== undefined) {
          await verify(phone, devAutoCode);
          return;
        }
        setState({
          stage: 'code',
          phone,
          pending: false,
          error: null,
          codeExpiresAt: expiresAt,
          roles: [],
        });
      } catch (error) {
        setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
      }
    },
    [client, devAutoCode, verify],
  );

  const submitCode = useCallback((code: string) => verify(state.phone, code), [verify, state.phone]);

  const restart = useCallback(() => setState(INITIAL), []);

  return useMemo(
    () => ({ ...state, requestCode, submitCode, restart }),
    [state, requestCode, submitCode, restart],
  );
}

/**
 * Turns an API error into something a person can act on.
 *
 * The server deliberately returns the same message whether or not an account
 * exists (document 28 forbids account enumeration), so this must not try to be
 * more specific than the server was.
 */
export function messageFor(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return 'Something went wrong. Please try again.';
  }
  switch (error.code) {
    case 'rate_limited':
      return 'Too many attempts. Please wait a few minutes and try again.';
    case 'unauthorized':
      return 'That code is incorrect or has expired.';
    case 'validation':
      return 'Please check the number and try again.';
    case 'unavailable':
      return 'We could not reach RideMe. Check your connection and try again.';
    case 'forbidden':
      return 'This account is not active. Please contact support.';
    default:
      return 'Something went wrong. Please try again.';
  }
}
