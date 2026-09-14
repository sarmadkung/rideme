/**
 * Shared mobile platform code.
 *
 * The customer and driver apps are two front doors onto one platform, and the
 * things they both need — signing in, storing a refresh token — belong here
 * rather than in either of them. Document 048 is explicit that business logic
 * must not be duplicated per platform; an auth flow copied into two apps
 * differs in exactly the ways nobody tests.
 *
 * Screens stay in the apps. Only what is genuinely identical lives here.
 */
export {
  useAuth,
  messageFor,
  DEV_OTP_BYPASS_CODE,
  type AuthActions,
  type AuthStage,
  type AuthState,
  type UseAuthOptions,
} from './useAuth.js';
export { secureTokenStorage } from './tokenStorage.js';
