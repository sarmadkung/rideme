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
// The login flow now lives in `@platform/auth`: it is not mobile-specific and
// a web client cannot import this package, which pulls in the device keystore.
// Re-exported so the apps' imports did not have to move with it.
export {
  useAuth,
  messageFor,
  type AuthActions,
  type AuthStage,
  type AuthState,
} from '@platform/auth';
export { secureTokenStorage } from './tokenStorage.js';
export {
  MutationQueue,
  DEFAULT_CAPACITY,
  type FlushResult,
  type QueueOptions,
  type QueueStorage,
  type QueuedMutation,
  type Sender,
} from './mutationQueue.js';
