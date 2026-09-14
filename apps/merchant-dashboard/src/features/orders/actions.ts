import { ApiError } from '@platform/api-client';
import type { GroceryOrderStatus, MerchantOrder, MerchantQueue } from '@platform/types';

/**
 * What a merchant may do to an order in a given state.
 *
 * This mirrors the guards in `internal/merchant/service.go` — Accept from
 * PLACED, Reject before preparation, Start Preparing from CONFIRMED, Mark
 * Ready from PREPARING, Report Issue only while picking. Document 77 asks the
 * UI to hide unavailable actions while the backend enforces them, and that is
 * what this is: a copy of the rule for *display*, never for authority. The
 * server still refuses everything it refused before, and the screen renders
 * whatever it says.
 */
export type OrderAction = 'accept' | 'reject' | 'preparing' | 'ready' | 'issue';

export function availableActions(status: string): OrderAction[] {
  switch (status) {
    case 'PLACED':
      return ['accept', 'reject'];
    // Accept is not offered: the answer is owed by the payment flow, not the
    // shop. Reject is, because a shop that knows it cannot fill the order
    // should not have to wait for a payment to succeed first.
    case 'PAYMENT_PENDING':
      return ['reject'];
    case 'CONFIRMED':
      return ['preparing', 'reject'];
    case 'PREPARING':
      return ['ready', 'issue'];
    default:
      return [];
  }
}

export const QUEUE_LABELS: Record<MerchantQueue, string> = {
  new: 'New',
  preparing: 'Preparing',
  ready: 'Ready',
  completed: 'Completed',
  cancelled: 'Cancelled',
};

const STATUS_LABELS: Record<GroceryOrderStatus, string> = {
  CART: 'Cart',
  PLACED: 'Awaiting your answer',
  PAYMENT_PENDING: 'Awaiting payment',
  CONFIRMED: 'Accepted',
  PREPARING: 'Being picked',
  READY_FOR_PICKUP: 'Waiting for a driver',
  PICKED_UP: 'Collected',
  DELIVERING: 'On its way',
  DELIVERED: 'Delivered',
  CANCELLED: 'Cancelled',
  FAILED: 'Failed',
};

/**
 * A status as a person reads it.
 *
 * Unknown values are shown as they arrived rather than hidden: a server that
 * has learned a state this build has not is a deployment fact the shop should
 * see, not one the screen should swallow.
 */
export function statusLabel(status: string): string {
  return STATUS_LABELS[status as GroceryOrderStatus] ?? status;
}

/**
 * How long the shop has left to answer, in whole minutes (BD-12).
 *
 * Returns null when the order is not waiting on an answer, and 0 once the
 * deadline has passed — an order the sweeper is about to cancel still shows
 * "0m", because "—" would read as "no hurry".
 */
export function minutesLeft(order: MerchantOrder, now: number): number | null {
  if (!order.accept_deadline) return null;
  const deadline = Date.parse(order.accept_deadline);
  if (Number.isNaN(deadline)) return null;
  return Math.max(0, Math.ceil((deadline - now) / 60_000));
}

/**
 * Turns a failed action into something a shop can act on.
 *
 * The conflicts are the interesting ones: they mean somebody or something else
 * moved the order — the customer cancelled it, the sweeper timed it out, a
 * colleague on the other terminal accepted it. The server's own message says
 * which, and it is written for this reader, so it is passed through rather
 * than replaced with a generic apology.
 */
export function describeFailure(error: unknown): string {
  if (!(error instanceof ApiError)) return 'Something went wrong. Please try again.';
  if (error.requiresLogin) return 'Your session has ended. Please sign in again.';
  switch (error.code) {
    case 'conflict':
    case 'validation':
    case 'forbidden':
      return error.message;
    case 'unavailable':
      return 'We could not reach RideMe. Check your connection and try again.';
    case 'not_found':
      return 'This order is no longer here. Reload the queue.';
    default:
      return 'Something went wrong. Please try again.';
  }
}
