import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, type ApiClient, type StopInput } from '@platform/api-client';
import type { CancelResult, Job, Quote } from '@platform/types';

/**
 * The customer booking flow from document 034: quote → confirm → track →
 * cancel.
 *
 * Like useAuth, this lives in shared TypeScript rather than in a screen. The
 * flow is identical on both platforms and a copy per platform differs in
 * exactly the ways nobody tests.
 */
export type BookingStage =
  /** Choosing pickup and destination. */
  | 'planning'
  /** A quote is in hand and the customer has not committed. */
  | 'quoted'
  /** A job exists and is being followed. */
  | 'tracking';

export interface BookingState {
  stage: BookingStage;
  pickup: StopInput | null;
  dropoff: StopInput | null;
  quote: Quote | null;
  job: Job | null;
  /** What the last cancellation cost, so the customer can be told. */
  cancellation: CancelResult | null;
  pending: boolean;
  error: string | null;
}

export interface BookingActions {
  setPickup(stop: StopInput | null): void;
  setDropoff(stop: StopInput | null): void;
  requestQuote(): Promise<void>;
  confirm(): Promise<void>;
  cancel(reason?: string): Promise<void>;
  reset(): void;
}

const INITIAL: BookingState = {
  stage: 'planning',
  pickup: null,
  dropoff: null,
  quote: null,
  job: null,
  cancellation: null,
  pending: false,
  error: null,
};

/** How often a tracked job is re-read while it is still live. */
export const TRACK_POLL_MS = 5000;

/**
 * Job states that are over. Polling stops here, and a finished job is not
 * cancellable.
 *
 * EXPIRED is BD-04's outcome — dispatch searched and found nobody. The
 * customer reaches it without being charged, which is why it sits alongside
 * COMPLETED rather than being treated as a failure to retry automatically.
 */
const FINISHED = new Set(['COMPLETED', 'CANCELLED', 'FAILED', 'EXPIRED', 'DISPUTED']);

export function isFinished(job: Job | null): boolean {
  return job !== null && FINISHED.has(job.status);
}

/** A job whose cancellation is still possible (document 036). */
export function isCancellable(job: Job | null): boolean {
  if (job === null) return false;
  return !FINISHED.has(job.status) && job.status !== 'IN_PROGRESS' && job.status !== 'AT_DROPOFF';
}

export function useBooking(
  client: ApiClient,
  options: { vehicleType?: string; city?: string; pollMs?: number } = {},
): BookingState & BookingActions {
  const { vehicleType = 'CAR', city, pollMs = TRACK_POLL_MS } = options;
  const [state, setState] = useState<BookingState>(INITIAL);

  // A ref mirror of state, so the callbacks below read current values without
  // being rebuilt on every change — and without the stale-closure bug that
  // reading `state` directly inside them would give.
  const stateRef = useRef(state);
  stateRef.current = state;

  // The idempotency key is generated once per quote and reused for every
  // confirm attempt on it. That is the entire point: a customer who taps
  // Confirm twice, or whose network retries, gets one job rather than two.
  const confirmKey = useRef<string | null>(null);

  const setPickup = useCallback((stop: StopInput | null) => {
    // Moving a pin invalidates the price it produced. Keeping a stale quote
    // visible would let a customer confirm a fare for a route they no longer
    // want.
    setState((s) => ({ ...s, pickup: stop, quote: null, stage: 'planning', error: null }));
    confirmKey.current = null;
  }, []);

  const setDropoff = useCallback((stop: StopInput | null) => {
    setState((s) => ({ ...s, dropoff: stop, quote: null, stage: 'planning', error: null }));
    confirmKey.current = null;
  }, []);

  const requestQuote = useCallback(async () => {
    // Read before writing. Validating inside a state updater would then have
    // to re-read the value it just checked, and setState is not synchronous —
    // so the check and the use would be looking at different things.
    const { pickup, dropoff } = stateRef.current;
    if (pickup === null || dropoff === null) {
      setState((s) => ({ ...s, error: 'Choose a pickup and a destination first.' }));
      return;
    }
    setState((s) => ({ ...s, pending: true, error: null }));

    try {
      const quote = await client.quote({
        jobType: 'RIDE',
        vehicleType,
        city,
        stops: [
          { ...pickup, type: 'PICKUP' },
          { ...dropoff, type: 'DROPOFF' },
        ],
      });
      confirmKey.current = newIdempotencyKey();
      setState((s) => ({ ...s, stage: 'quoted', quote, pending: false, error: null }));
    } catch (error) {
      setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
    }
  }, [client, vehicleType, city]);

  const confirm = useCallback(async () => {
    const { quote, pickup, dropoff } = stateRef.current;
    if (quote === null || pickup === null || dropoff === null) return;

    const key = confirmKey.current ?? newIdempotencyKey();
    confirmKey.current = key;
    setState((s) => ({ ...s, pending: true, error: null }));

    try {
      const job = await client.createJob(
        {
          quoteId: quote.quote_id,
          jobType: 'RIDE',
          stops: [
            { ...pickup, type: 'PICKUP' },
            { ...dropoff, type: 'DROPOFF' },
          ],
        },
        key,
      );
      setState((s) => ({ ...s, stage: 'tracking', job, pending: false, error: null }));
    } catch (error) {
      setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
    }
  }, [client]);

  const cancel = useCallback(
    async (reason?: string) => {
      const { job } = stateRef.current;
      if (job === null) return;
      setState((s) => ({ ...s, pending: true, error: null }));
      try {
        const result = await client.cancelJob(job.id, reason);
        setState((s) => ({
          ...s,
          job: result.job,
          cancellation: result,
          pending: false,
          error: null,
        }));
      } catch (error) {
        setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
      }
    },
    [client],
  );

  const reset = useCallback(() => {
    confirmKey.current = null;
    setState(INITIAL);
  }, []);

  // Follow a live job. Polling rather than a socket: the realtime gateway
  // exists but the client transport for it does not, and a customer watching a
  // stale screen is a worse failure than a request every few seconds.
  useEffect(() => {
    if (state.stage !== 'tracking' || state.job === null || isFinished(state.job)) return;

    let cancelled = false;
    const id = setInterval(() => {
      void (async () => {
        const current = stateRef.current.job;
        if (current === null) return;
        try {
          const fresh = await client.getJob(current.id);
          if (!cancelled) setState((s) => ({ ...s, job: fresh }));
        } catch {
          // A failed poll is not worth surfacing: the next one is seconds
          // away, and an error banner that flickers on every dropped request
          // teaches the customer to ignore error banners.
        }
      })();
    }, pollMs);

    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [client, pollMs, state.stage, state.job?.id, state.job?.status]);

  return { ...state, setPickup, setDropoff, requestQuote, confirm, cancel, reset };
}

/**
 * A key unique enough that two taps never collide.
 *
 * crypto.randomUUID is not available on every React Native runtime, so this
 * does not depend on it. Collision resistance here only needs to hold within
 * one user's requests, where time plus 64 bits of randomness is ample.
 */
function newIdempotencyKey(): string {
  const random = () => Math.floor(Math.random() * 0xffffffff).toString(16);
  return `${Date.now().toString(16)}-${random()}-${random()}`;
}

function messageFor(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return 'Something went wrong. Please try again.';
}
