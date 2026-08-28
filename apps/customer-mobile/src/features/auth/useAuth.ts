import { useCallback, useMemo, useState } from 'react';
import { ApiError, type ApiClient } from '@platform/api-client';

/**
 * The phone-OTP login flow from document 28, as app state.
 *
 * It lives in shared TypeScript rather than in a screen, and there is one copy
 * for both platforms: document 48 is explicit that business logic must not be
 * duplicated per platform, and an auth flow that differs between iOS and
 * Android differs in exactly the ways nobody tests.
 */
export type AuthStage = 'phone' | 'code' | 'authenticated';

export interface AuthState {
  stage: AuthStage;
  phone: string;
  pending: boolean;
  error: string | null;
  /** When the current code expires, so the UI can offer a resend. */
  codeExpiresAt: Date | null;
}

export interface AuthActions {
  requestCode(phone: string): Promise<void>;
  submitCode(code: string): Promise<boolean>;
  restart(): void;
}

const INITIAL: AuthState = {
  stage: 'phone',
  phone: '',
  pending: false,
  error: null,
  codeExpiresAt: null,
};

export function useAuth(client: ApiClient): AuthState & AuthActions {
  const [state, setState] = useState<AuthState>(INITIAL);

  const requestCode = useCallback(
    async (phone: string) => {
      setState((s) => ({ ...s, pending: true, error: null }));
      try {
        const { expiresAt } = await client.requestOtp(phone);
        setState({
          stage: 'code',
          phone,
          pending: false,
          error: null,
          codeExpiresAt: expiresAt,
        });
      } catch (error) {
        setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
      }
    },
    [client],
  );

  const submitCode = useCallback(
    async (code: string) => {
      setState((s) => ({ ...s, pending: true, error: null }));
      try {
        await client.verifyOtp(state.phone, code);
        setState((s) => ({ ...s, stage: 'authenticated', pending: false, error: null }));
        return true;
      } catch (error) {
        setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
        return false;
      }
    },
    [client, state.phone],
  );

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
