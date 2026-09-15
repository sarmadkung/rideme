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

/** Where the driver is, when the gateway has told us. */
export interface DriverPosition {
  latitude: number;
  longitude: number;
  recordedAt: string;
}

export interface BookingState {
  stage: BookingStage;
  /**
   * The driver's last known position, or null when nothing has arrived. Comes
   * from the realtime stream only: the platform will not let a customer read a
   * driver's position on demand outside their own active job, and the job
   * channel is that scoping expressed as a subscription.
   */
  driverPosition: DriverPosition | null;
  /** Whether the realtime stream is currently connected. */
  live: boolean;
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
  driverPosition: null,
  live: false,
  pickup: null,
  dropoff: null,
  quote: null,
  job: null,
  cancellation: null,
  pending: false,
  error: null,
};

/**
 * How often a tracked job is re-read while it is still live.
 *
 * Thirty seconds rather than five. This used to be the only way a customer's
 * screen changed; now the realtime stream delivers status changes as they
 * happen and this is the safety net underneath it — for a stream that dropped
 * without reporting it, or a platform where the socket cannot be held open at
 * all. Polling this slowly with no stream is a worse experience than before,
 * and polling every five seconds with one is a request per customer per five
 * seconds for information that already arrived.
 */
export const TRACK_POLL_MS = 30_000;

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

  // Watch a live job over the realtime gateway.
  //
  // A status event carries the new status, but the screen is re-fetched rather
  // than patched from the payload: the gateway is not the system of record
  // (document 047), and a job assembled from events would drift from the one
  // the server holds the first time an event is dropped under backpressure —
  // which that gateway does deliberately.
  useEffect(() => {
    if (state.stage !== 'tracking' || state.job === null || isFinished(state.job)) return;

    const jobId = state.job.id;
    let cancelled = false;

    const refetch = () => {
      void (async () => {
        try {
          const fresh = await client.getJob(jobId);
          if (!cancelled) setState((s) => (s.job?.id === jobId ? { ...s, job: fresh } : s));
        } catch {
          // Same reasoning as the poll below: a failed read is not worth a
          // banner when another is coming.
        }
      })();
    };

    const stream = client.watchJob(jobId, {
      onOpen: () => {
        if (cancelled) return;
        setState((s) => ({ ...s, live: true }));
        // Every connection, not just reconnections. The gateway does not
        // replay, so whatever happened while there was no stream is recovered
        // by asking — including anything between confirming the job and this
        // subscription being accepted.
        refetch();
      },
      onEvent: (event) => {
        if (cancelled) return;
        if (event.type === 'job.status_changed') {
          refetch();
          return;
        }
        if (event.type === 'driver.location') {
          const payload = event.payload as {
            latitude?: number;
            longitude?: number;
            recorded_at?: string;
          };
          if (typeof payload.latitude !== 'number' || typeof payload.longitude !== 'number') return;
          setState((s) => ({
            ...s,
            driverPosition: {
              latitude: payload.latitude as number,
              longitude: payload.longitude as number,
              recordedAt: payload.recorded_at ?? new Date().toISOString(),
            },
          }));
        }
      },
      onError: () => {
        if (!cancelled) setState((s) => ({ ...s, live: false }));
      },
    });

    return () => {
      cancelled = true;
      stream.close();
      setState((s) => ({ ...s, live: false }));
    };
  }, [client, state.stage, state.job?.id]);

  // The safety net under the stream.
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
