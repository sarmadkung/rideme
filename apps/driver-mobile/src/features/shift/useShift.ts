import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, type ApiClient, type PositionInput } from '@platform/api-client';
import type { DriverAssignment, DriverProfile, Job } from '@platform/types';

/**
 * A driver's working session: go online, take an offer, drive the trip.
 *
 * The state here mirrors what the server already knows rather than tracking a
 * parallel copy of it. A driver app that decides locally what stage a trip is
 * at will disagree with the server the first time a request fails, and the
 * disagreement shows up as a driver unable to complete a job they have
 * finished.
 */
export interface ShiftState {
  driver: DriverProfile | null;
  /** The offer or trip being held, or null when idle. */
  assignment: DriverAssignment | null;
  pending: boolean;
  error: string | null;
  /** Seconds left to answer an offer, or null when there is nothing to answer. */
  offerSecondsLeft: number | null;
}

export interface ShiftActions {
  refresh(): Promise<void>;
  goOnline(at: PositionInput): Promise<void>;
  goOffline(): Promise<void>;
  accept(): Promise<void>;
  reject(): Promise<void>;
  advance(): Promise<void>;
  report(at: PositionInput): Promise<void>;
}

const INITIAL: ShiftState = {
  driver: null,
  assignment: null,
  pending: false,
  error: null,
  offerSecondsLeft: null,
};

/** How often an online driver checks for work. */
export const POLL_MS = 4000;

export function isOnline(driver: DriverProfile | null): boolean {
  if (driver === null) return false;
  return driver.status !== 'OFFLINE' && driver.status !== 'SUSPENDED' && driver.status !== 'BLOCKED';
}

/**
 * The next command for a job in progress, or null when there is none.
 *
 * The sequence is document 035's, and it is deliberately one step at a time.
 * A driver holding a job in ACCEPTED can only arrive; one at the pickup can
 * only start. Offering more would let a driver complete a trip they never
 * began.
 */
export function nextCommand(job: Job | null): { action: 'arrive' | 'start' | 'complete'; label: string } | null {
  if (job === null) return null;
  switch (job.status) {
    case 'ACCEPTED':
    case 'ARRIVING':
      return { action: 'arrive', label: "I've arrived" };
    case 'AT_PICKUP':
      return { action: 'start', label: 'Start trip' };
    case 'IN_PROGRESS':
    case 'AT_DROPOFF':
      return { action: 'complete', label: 'Complete trip' };
    default:
      return null;
  }
}

/** An assignment the driver has not yet answered. */
export function isOffer(assignment: DriverAssignment | null): boolean {
  return assignment !== null && assignment.status === 'OFFERED';
}

export function useShift(
  client: ApiClient,
  options: { pollMs?: number; now?: () => number } = {},
): ShiftState & ShiftActions {
  const { pollMs = POLL_MS, now = () => Date.now() } = options;
  const [state, setState] = useState<ShiftState>(INITIAL);

  const stateRef = useRef(state);
  stateRef.current = state;

  const run = useCallback(async (work: () => Promise<Partial<ShiftState>>) => {
    setState((s) => ({ ...s, pending: true, error: null }));
    try {
      const patch = await work();
      setState((s) => ({ ...s, ...patch, pending: false, error: null }));
    } catch (error) {
      setState((s) => ({ ...s, pending: false, error: messageFor(error) }));
    }
  }, []);

  const refresh = useCallback(async () => {
    // Through run() like every other action. This is called on mount, and a
    // signed-in account that is not a driver makes driverMe throw — outside
    // run() that surfaces as an unhandled rejection rather than as a message
    // the driver can read.
    //
    // Both reads happen together. Reading only one leaves the app showing an
    // offer for a driver who has gone offline, or an online driver with no
    // sign of the job they are holding.
    await run(async () => {
      const [driver, assignment] = await Promise.all([
        client.driverMe(),
        client.driverAssignment(),
      ]);
      return { driver, assignment };
    });
  }, [client, run]);

  const goOnline = useCallback(
    async (at: PositionInput) => {
      await run(async () => {
        const driver = await client.goOnline(at);
        return { driver };
      });
    },
    [client, run],
  );

  const goOffline = useCallback(async () => {
    await run(async () => {
      const driver = await client.goOffline();
      // Going offline ends any claim on work. Keeping a stale assignment
      // visible would offer commands the server will refuse.
      return { driver, assignment: null };
    });
  }, [client, run]);

  const accept = useCallback(async () => {
    const assignment = stateRef.current.assignment;
    if (assignment === null) return;
    await run(async () => {
      await client.driverAccept(assignment.job.id);
      const [driver, fresh] = await Promise.all([client.driverMe(), client.driverAssignment()]);
      return { driver, assignment: fresh };
    });
  }, [client, run]);

  const reject = useCallback(async () => {
    const assignment = stateRef.current.assignment;
    if (assignment === null) return;
    await run(async () => {
      await client.driverReject(assignment.job.id);
      const [driver, fresh] = await Promise.all([client.driverMe(), client.driverAssignment()]);
      return { driver, assignment: fresh };
    });
  }, [client, run]);

  const advance = useCallback(async () => {
    const assignment = stateRef.current.assignment;
    const next = nextCommand(assignment?.job ?? null);
    if (assignment === null || next === null) return;

    await run(async () => {
      if (next.action === 'arrive') await client.driverArrive(assignment.job.id);
      else if (next.action === 'start') await client.driverStart(assignment.job.id);
      else await client.driverComplete(assignment.job.id);

      // The server's view is authoritative. Patching the job locally would
      // drift the moment a command partly succeeded.
      const [driver, fresh] = await Promise.all([client.driverMe(), client.driverAssignment()]);
      return { driver, assignment: fresh };
    });
  }, [client, run]);

  const report = useCallback(
    async (at: PositionInput) => {
      try {
        await client.reportLocation([at]);
      } catch {
        // A dropped position report is not worth an error banner. The next one
        // is seconds away, and a banner that flickers teaches drivers to
        // ignore banners.
      }
    },
    [client],
  );

  // Poll for work while online. An offline driver is not waiting for anything,
  // so nothing is polled — which also means a driver who signs off stops
  // spending battery on requests.
  useEffect(() => {
    if (!isOnline(state.driver)) return;

    let cancelled = false;
    const id = setInterval(() => {
      void (async () => {
        try {
          const [driver, assignment] = await Promise.all([
            client.driverMe(),
            client.driverAssignment(),
          ]);
          if (!cancelled) setState((s) => ({ ...s, driver, assignment }));
        } catch {
          // Same reasoning as report: the next poll is seconds away.
        }
      })();
    }, pollMs);

    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [client, pollMs, state.driver?.status]);

  // The offer countdown, driven from the server's expiry rather than from a
  // local TTL. A countdown that disagrees with the server would let a driver
  // tap Accept on an offer that has already gone.
  const expiresAt = state.assignment?.expires_at ?? null;
  const isOfferPending = isOffer(state.assignment);
  useEffect(() => {
    if (!isOfferPending || expiresAt === undefined || expiresAt === null) {
      setState((s) => (s.offerSecondsLeft === null ? s : { ...s, offerSecondsLeft: null }));
      return;
    }

    const deadline = new Date(expiresAt).getTime();
    const tick = () => {
      const left = Math.max(0, Math.ceil((deadline - now()) / 1000));
      setState((s) => (s.offerSecondsLeft === left ? s : { ...s, offerSecondsLeft: left }));
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [expiresAt, isOfferPending, now]);

  return { ...state, refresh, goOnline, goOffline, accept, reject, advance, report };
}

function messageFor(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return 'Something went wrong. Please try again.';
}
