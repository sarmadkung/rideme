import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, type ApiClient, type DeliveryInput } from '@platform/api-client';
import type {
  GroceryOrder,
  OrderIssue,
  Product,
  Store,
  SubstitutionPreference,
} from '@platform/types';

/**
 * The customer's grocery flow (documents 68, 70, 71): find a shop → fill a
 * cart → say where it goes → follow it to the door.
 *
 * Like `useBooking`, the flow lives here rather than in a screen. Nothing in
 * this file imports React Native: it is the same logic on both platforms, and
 * it is testable without rendering anything.
 */
export type GroceryStage =
  /** Choosing a shop. */
  | 'stores'
  /** Inside one shop, filling a cart. */
  | 'shopping'
  /** Saying where it goes. */
  | 'checkout'
  /** Placed, and being followed. */
  | 'tracking';

export interface GroceryState {
  stage: GroceryStage;
  stores: Store[];
  store: Store | null;
  products: Product[];
  order: GroceryOrder | null;
  pending: boolean;
  error: string | null;
}

export interface GroceryActions {
  findStores(near: { latitude: number; longitude: number }): Promise<void>;
  enterStore(store: Store): Promise<void>;
  addItem(
    product: Product,
    quantity: number,
    preference?: SubstitutionPreference,
    variantId?: string,
  ): Promise<void>;
  toCheckout(): void;
  backToShopping(): void;
  place(delivery: DeliveryInput): Promise<void>;
  decide(issueId: string, accept: boolean): Promise<void>;
  cancel(reason?: string): Promise<void>;
  reset(): void;
}

const INITIAL: GroceryState = {
  stage: 'stores',
  stores: [],
  store: null,
  products: [],
  order: null,
  pending: false,
  error: null,
};

/** How often a placed order is re-read while it is still live. */
export const ORDER_POLL_MS = 8000;

/**
 * States that are over (document 70). Polling stops here.
 *
 * Slower than the ride poll at five seconds, deliberately: a grocery order
 * spends most of its life being picked, which is minutes of a human walking
 * aisles, not a car moving on a map.
 */
const FINISHED = new Set(['DELIVERED', 'CANCELLED', 'FAILED']);

export function isFinished(order: GroceryOrder | null): boolean {
  return order !== null && FINISHED.has(order.status);
}

/**
 * Whether the customer may still call it off.
 *
 * This mirrors `CustomerCancellable` in the service: once a picker has walked
 * the aisles, cancelling wastes goods somebody handled, and that is a
 * conversation with support rather than a tap. Offering the button anyway
 * would produce a refusal the customer can do nothing about.
 */
export function isCancellable(order: GroceryOrder | null): boolean {
  if (order === null) return false;
  return ['CART', 'PLACED', 'PAYMENT_PENDING', 'CONFIRMED'].includes(order.status);
}

/**
 * The question waiting on this customer, if there is one.
 *
 * `ASK_ME` is the only preference that produces one: the shop proposed a
 * substitute and changed nothing, and the customer owes what they ordered
 * until they answer. Everything else was already resolved server-side.
 */
export function pendingIssue(order: GroceryOrder | null): OrderIssue | null {
  return (order?.issues ?? []).find((issue) => issue.resolution === 'PENDING') ?? null;
}

export function useGrocery(
  client: ApiClient,
  options: { pollMs?: number } = {},
): GroceryState & GroceryActions {
  const { pollMs = ORDER_POLL_MS } = options;
  const [state, setState] = useState<GroceryState>(INITIAL);

  // A ref mirror, so the callbacks read current values without being rebuilt
  // on every change and without the stale-closure bug that reading `state`
  // inside them would give. Same reason as `useBooking`.
  const stateRef = useRef(state);
  stateRef.current = state;

  const run = useCallback(async <T>(work: () => Promise<T>, onDone: (result: T) => void) => {
    setState((s) => ({ ...s, pending: true, error: null }));
    try {
      const result = await work();
      onDone(result);
      setState((s) => ({ ...s, pending: false }));
    } catch (error) {
      setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
    }
  }, []);

  const findStores = useCallback(
    async (near: { latitude: number; longitude: number }) => {
      await run(
        () => client.listStores(near),
        (stores) => setState((s) => ({ ...s, stage: 'stores', stores })),
      );
    },
    [client, run],
  );

  const enterStore = useCallback(
    async (store: Store) => {
      await run(
        async () => {
          // The catalogue and the cart together: a shop with nothing to show
          // is not worth opening a cart for, and a cart opened later would
          // lose the first tap on "add".
          const [products, order] = await Promise.all([
            client.storeCatalog(store.id),
            client.openCart(store.id),
          ]);
          return { products, order };
        },
        ({ products, order }) =>
          setState((s) => ({ ...s, stage: 'shopping', store, products, order })),
      );
    },
    [client, run],
  );

  const addItem = useCallback(
    async (
      product: Product,
      quantity: number,
      preference?: SubstitutionPreference,
      variantId?: string,
    ) => {
      const order = stateRef.current.order;
      if (!order) return;
      await run(
        () =>
          client.addCartItem(order.id, {
            productId: product.id,
            quantity,
            ...(preference ? { substitutionPreference: preference } : {}),
            ...(variantId ? { variantId } : {}),
          }),
        // The server returns the cart, total included. A client that added the
        // line itself would be computing money, which it must never do.
        (updated) => setState((s) => ({ ...s, order: updated })),
      );
    },
    [client, run],
  );

  const place = useCallback(
    async (delivery: DeliveryInput) => {
      const order = stateRef.current.order;
      if (!order) return;
      await run(
        () => client.placeGroceryOrder(order.id, delivery),
        (placed) => setState((s) => ({ ...s, stage: 'tracking', order: placed })),
      );
    },
    [client, run],
  );

  const decide = useCallback(
    async (issueId: string, accept: boolean) => {
      const order = stateRef.current.order;
      if (!order) return;
      await run(
        () => client.decideGroceryIssue(order.id, issueId, accept),
        (updated) => setState((s) => ({ ...s, order: updated })),
      );
    },
    [client, run],
  );

  const cancel = useCallback(
    async (reason?: string) => {
      const order = stateRef.current.order;
      if (!order) return;
      await run(
        () => client.cancelGroceryOrder(order.id, reason),
        (cancelled) => setState((s) => ({ ...s, order: cancelled })),
      );
    },
    [client, run],
  );

  const toCheckout = useCallback(() => setState((s) => ({ ...s, stage: 'checkout' })), []);
  const backToShopping = useCallback(() => setState((s) => ({ ...s, stage: 'shopping' })), []);
  const reset = useCallback(() => setState(INITIAL), []);

  // Follow a placed order. The shop accepts it, picks it, reports what it
  // could not find and hands it to a driver, and none of that is something
  // the customer's app is told about — there is no socket yet.
  useEffect(() => {
    if (state.stage !== 'tracking' || pollMs <= 0) return;
    const order = state.order;
    if (!order || isFinished(order)) return;

    const timer = setInterval(() => {
      void client
        .getGroceryOrder(order.id)
        .then((fresh) => setState((s) => ({ ...s, order: fresh })))
        // A failed poll is not worth a message: the next one is eight seconds
        // away, and an error banner that clears itself teaches people to
        // ignore error banners.
        .catch(() => undefined);
    }, pollMs);
    return () => clearInterval(timer);
  }, [client, pollMs, state.stage, state.order]);

  return {
    ...state,
    findStores,
    enterStore,
    addItem,
    toCheckout,
    backToShopping,
    place,
    decide,
    cancel,
    reset,
  };
}

/**
 * Turns an API error into something a customer can act on.
 *
 * The conflicts carry the server's own sentence — "there is nothing in this
 * cart yet", "already being prepared — contact support to cancel it" — and
 * those are written for this reader. Replacing them with a generic apology
 * would lose the only part that says what to do next.
 */
export function messageFor(error: unknown): string {
  if (!(error instanceof ApiError)) return 'Something went wrong. Please try again.';
  switch (error.code) {
    case 'conflict':
    case 'validation':
      return error.message;
    case 'unavailable':
      return 'We could not reach RideMe. Check your connection and try again.';
    case 'not_found':
      return 'That is no longer available.';
    case 'unauthorized':
      return 'Your session has ended. Please sign in again.';
    default:
      return 'Something went wrong. Please try again.';
  }
}
