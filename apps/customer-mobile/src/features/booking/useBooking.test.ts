import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError, type ApiClient } from '@platform/api-client';
import type { Job, Quote } from '@platform/types';
import { isCancellable, isFinished, TRACK_POLL_MS, useBooking } from './useBooking';

const PICKUP = { latitude: 31.5204, longitude: 74.3587 };
const DROPOFF = { latitude: 31.588, longitude: 74.315 };

function aQuote(overrides: Partial<Quote> = {}): Quote {
  return {
    quote_id: 'q-1',
    total: { amount_minor: 45000, currency: 'PKR' },
    lines: [
      { component: 'base', amount: { amount_minor: 5000, currency: 'PKR' } },
      { component: 'distance', amount: { amount_minor: 40000, currency: 'PKR' } },
    ],
    distance_meters: 8200,
    duration_seconds: 1080,
    route_confidence: 'ESTIMATED',
    expires_at: '2026-08-29T12:05:00Z',
    ...overrides,
  } as Quote;
}

function aJob(status = 'REQUESTED', overrides: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    type: 'RIDE',
    status,
    stops: [],
    created_at: '2026-08-29T12:00:00Z',
    ...overrides,
  } as Job;
}

/**
 * The handlers the hook passed to watchJob, so a test can deliver events the
 * way the gateway would. One entry per subscription; the last is the live one.
 */
const subscriptions: Array<{
  jobId: string;
  handlers: {
    onEvent(event: { type: string; payload: unknown }): void;
    onOpen?(): void;
    onError?(error: Error): void;
  };
  closed: boolean;
}> = [];

function latestSubscription() {
  const subscription = subscriptions[subscriptions.length - 1];
  if (subscription === undefined) throw new Error('nothing subscribed to the realtime gateway');
  return subscription;
}

beforeEach(() => {
  subscriptions.length = 0;
});

function stubClient(overrides: Record<string, unknown> = {}) {
  return {
    watchJob: jest.fn((jobId: string, handlers) => {
      const subscription = { jobId, handlers, closed: false };
      subscriptions.push(subscription);
      return {
        close: () => {
          subscription.closed = true;
        },
      };
    }),
    quote: jest.fn(async () => aQuote()),
    createJob: jest.fn(async () => aJob()),
    getJob: jest.fn(async () => aJob()),
    cancelJob: jest.fn(async () => ({
      job: aJob('CANCELLED'),
      cancellation_tier: 'BEFORE_ASSIGNMENT',
      fee: { amount_minor: 0, currency: 'PKR' },
    })),
    ...overrides,
  };
}

/** The stub is structurally a client; the cast is at the boundary, not inside
 * the stub, so the mocks keep their jest.Mock types for assertions. */
function asClient(stub: ReturnType<typeof stubClient>): ApiClient {
  return stub as unknown as ApiClient;
}

async function planned(client: ReturnType<typeof stubClient>) {
  const rendered = renderHook(() => useBooking(asClient(client)));
  act(() => {
    rendered.result.current.setPickup(PICKUP);
    rendered.result.current.setDropoff(DROPOFF);
  });
  return rendered;
}

describe('useBooking', () => {
  it('walks planning → quoted → tracking', async () => {
    const client = stubClient();
    const { result } = await planned(client);
    expect(result.current.stage).toBe('planning');

    await act(async () => {
      await result.current.requestQuote();
    });
    expect(result.current.stage).toBe('quoted');
    expect(result.current.quote?.total.amount_minor).toBe(45000);

    await act(async () => {
      await result.current.confirm();
    });
    expect(result.current.stage).toBe('tracking');
    expect(result.current.job?.id).toBe('job-1');
  });

  it('refuses to quote without both ends of the trip', async () => {
    const client = stubClient();
    const { result } = renderHook(() => useBooking(asClient(client)));

    act(() => {
      result.current.setPickup(PICKUP);
    });
    await act(async () => {
      await result.current.requestQuote();
    });

    expect(result.current.error).toMatch(/pickup and a destination/i);
    expect(client.quote).not.toHaveBeenCalled();
  });

  it('drops a quote when the route changes', async () => {
    // Keeping it would let a customer confirm a fare for a trip they no longer
    // want to take.
    const client = stubClient();
    const { result } = await planned(client);
    await act(async () => {
      await result.current.requestQuote();
    });
    expect(result.current.quote).not.toBeNull();

    act(() => {
      result.current.setDropoff({ latitude: 31.4, longitude: 74.2 });
    });

    expect(result.current.quote).toBeNull();
    expect(result.current.stage).toBe('planning');
  });

  it('sends one idempotency key however many times confirm is tapped', async () => {
    // The whole reason the key exists: a double tap, or a network retry, must
    // produce one ride rather than two.
    const client = stubClient();
    const { result } = await planned(client);
    await act(async () => {
      await result.current.requestQuote();
    });

    await act(async () => {
      await Promise.all([result.current.confirm(), result.current.confirm()]);
    });

    const keys = (client.createJob as jest.Mock).mock.calls.map((call) => call[1]);
    expect(keys.length).toBeGreaterThan(1);
    expect(new Set(keys).size).toBe(1);
  });

  it('uses a fresh key for a new quote', async () => {
    // A different trip is a different request. Reusing the key would make the
    // server return the first ride instead of booking the second.
    const client = stubClient();
    const { result } = await planned(client);

    await act(async () => {
      await result.current.requestQuote();
    });
    await act(async () => {
      await result.current.confirm();
    });
    const first = (client.createJob as jest.Mock).mock.calls[0][1];

    act(() => {
      result.current.setDropoff({ latitude: 31.4, longitude: 74.2 });
    });
    await act(async () => {
      await result.current.requestQuote();
    });
    await act(async () => {
      await result.current.confirm();
    });
    const second = (client.createJob as jest.Mock).mock.calls[1][1];

    expect(second).not.toBe(first);
  });

  it('reports what a cancellation cost', async () => {
    // BD-01 charges after the free window. A customer must be told the amount,
    // not left to find it on a statement.
    const client = stubClient({
      cancelJob: jest.fn(async () => ({
        job: aJob('CANCELLED'),
        cancellation_tier: 'AFTER_ASSIGNMENT',
        fee: { amount_minor: 10000, currency: 'PKR' },
      })),
    });
    const { result } = await planned(client);
    await act(async () => {
      await result.current.requestQuote();
    });
    await act(async () => {
      await result.current.confirm();
    });
    await act(async () => {
      await result.current.cancel('changed my mind');
    });

    expect(result.current.cancellation?.fee.amount_minor).toBe(10000);
    expect(result.current.job?.status).toBe('CANCELLED');
  });

  it('surfaces the server message when quoting fails', async () => {
    const client = stubClient({
      quote: jest.fn(async () => {
        throw new ApiError(503, {
          code: 'unavailable',
          message: 'this service is not available here yet',
          request_id: 'r',
        });
      }),
    });
    const { result } = await planned(client);
    await act(async () => {
      await result.current.requestQuote();
    });

    expect(result.current.error).toBe('this service is not available here yet');
    expect(result.current.stage).toBe('planning');
  });

  it('polls a live job and stops once it finishes', async () => {
    jest.useFakeTimers();
    try {
      const getJob = jest
        .fn()
        .mockResolvedValueOnce(aJob('SEARCHING'))
        .mockResolvedValueOnce(aJob('EXPIRED'));
      const client = stubClient({ getJob });
      const { result } = await planned(client);

      await act(async () => {
        await result.current.requestQuote();
      });
      await act(async () => {
        await result.current.confirm();
      });

      await act(async () => {
        jest.advanceTimersByTime(TRACK_POLL_MS);
      });
      await waitFor(() => expect(result.current.job?.status).toBe('SEARCHING'));

      await act(async () => {
        jest.advanceTimersByTime(TRACK_POLL_MS);
      });
      await waitFor(() => expect(result.current.job?.status).toBe('EXPIRED'));

      // BD-04's outcome is terminal. Polling a job nothing will move again is
      // a request forever, at whatever interval.
      const callsAtRest = getJob.mock.calls.length;
      await act(async () => {
        jest.advanceTimersByTime(TRACK_POLL_MS * 4);
      });
      expect(getJob.mock.calls.length).toBe(callsAtRest);
    } finally {
      jest.useRealTimers();
    }
  });

  describe('the realtime stream', () => {
    async function tracking(client: ReturnType<typeof stubClient>) {
      const { result } = await planned(client);
      await act(async () => {
        await result.current.requestQuote();
      });
      await act(async () => {
        await result.current.confirm();
      });
      return result;
    }

    it('subscribes to the job it is tracking', async () => {
      const client = stubClient();
      const result = await tracking(client);

      expect(subscriptions).toHaveLength(1);
      expect(latestSubscription().jobId).toBe(result.current.job?.id);
    });

    // The gateway does not replay (document 047), so whatever happened while
    // there was no stream is recovered by asking — including anything between
    // confirming the job and the subscription being accepted.
    it('refetches the job on every connection, and reports being live', async () => {
      const getJob = jest.fn(async () => aJob('ACCEPTED'));
      const client = stubClient({ getJob });
      const result = await tracking(client);

      await act(async () => {
        latestSubscription().handlers.onOpen?.();
      });

      await waitFor(() => expect(result.current.job?.status).toBe('ACCEPTED'));
      expect(result.current.live).toBe(true);
    });

    // The screen is re-fetched rather than patched from the event payload: the
    // gateway drops events under backpressure by design, and a job assembled
    // from events would drift from the one the server holds.
    it('refetches when a status event arrives', async () => {
      const getJob = jest.fn(async () => aJob('AT_PICKUP'));
      const client = stubClient({ getJob });
      const result = await tracking(client);
      const before = getJob.mock.calls.length;

      await act(async () => {
        latestSubscription().handlers.onEvent({
          type: 'job.status_changed',
          payload: { status: 'AT_PICKUP' },
        });
      });

      await waitFor(() => expect(result.current.job?.status).toBe('AT_PICKUP'));
      expect(getJob.mock.calls.length).toBeGreaterThan(before);
    });

    it("keeps the driver's last known position", async () => {
      const client = stubClient();
      const result = await tracking(client);

      await act(async () => {
        latestSubscription().handlers.onEvent({
          type: 'driver.location',
          payload: { latitude: 31.5204, longitude: 74.3587, recorded_at: '2026-09-15T09:00:00Z' },
        });
      });

      await waitFor(() => expect(result.current.driverPosition).not.toBeNull());
      expect(result.current.driverPosition?.latitude).toBeCloseTo(31.5204);
      expect(result.current.driverPosition?.longitude).toBeCloseTo(74.3587);
    });

    // A malformed payload must not blank a position the customer is watching.
    it('ignores a position with no coordinates', async () => {
      const client = stubClient();
      const result = await tracking(client);

      await act(async () => {
        latestSubscription().handlers.onEvent({
          type: 'driver.location',
          payload: { latitude: 31.5204, longitude: 74.3587 },
        });
      });
      await waitFor(() => expect(result.current.driverPosition).not.toBeNull());

      await act(async () => {
        latestSubscription().handlers.onEvent({ type: 'driver.location', payload: {} });
      });
      expect(result.current.driverPosition?.latitude).toBeCloseTo(31.5204);
    });

    // A stream that reported an error is not delivering events, and a screen
    // that still says "live" is lying to the customer about how fresh it is.
    it('stops claiming to be live when the stream errors', async () => {
      const client = stubClient();
      const result = await tracking(client);

      await act(async () => {
        latestSubscription().handlers.onOpen?.();
      });
      expect(result.current.live).toBe(true);

      await act(async () => {
        latestSubscription().handlers.onError?.(new Error('refused'));
      });
      expect(result.current.live).toBe(false);
    });

    // A stream left open after the customer leaves the screen is a connection
    // per abandoned trip, against a per-account limit of ten.
    it('closes the stream when tracking ends', async () => {
      const client = stubClient();
      const result = await tracking(client);
      const subscription = latestSubscription();

      await act(async () => {
        result.current.reset();
      });

      await waitFor(() => expect(subscription.closed).toBe(true));
    });
  });
});

describe('isFinished', () => {
  it('treats BD-04 expiry as over', () => {
    // A search that found nobody is finished, not still running.
    expect(isFinished(aJob('EXPIRED'))).toBe(true);
    expect(isFinished(aJob('COMPLETED'))).toBe(true);
    expect(isFinished(aJob('SEARCHING'))).toBe(false);
    expect(isFinished(null)).toBe(false);
  });
});

describe('isCancellable', () => {
  it('follows document 036 — a trip in progress is not cancellable', () => {
    expect(isCancellable(aJob('SEARCHING'))).toBe(true);
    expect(isCancellable(aJob('ACCEPTED'))).toBe(true);
    expect(isCancellable(aJob('AT_PICKUP'))).toBe(true);
    expect(isCancellable(aJob('IN_PROGRESS'))).toBe(false);
    expect(isCancellable(aJob('COMPLETED'))).toBe(false);
  });
});
