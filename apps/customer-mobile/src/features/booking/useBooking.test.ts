import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError, type ApiClient } from '@platform/api-client';
import type { Job, Quote } from '@platform/types';
import { isCancellable, isFinished, useBooking } from './useBooking';

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

function stubClient(overrides: Record<string, unknown> = {}) {
  return {
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
        jest.advanceTimersByTime(5000);
      });
      await waitFor(() => expect(result.current.job?.status).toBe('SEARCHING'));

      await act(async () => {
        jest.advanceTimersByTime(5000);
      });
      await waitFor(() => expect(result.current.job?.status).toBe('EXPIRED'));

      // BD-04's outcome is terminal. Polling a job nothing will move again is
      // a request every five seconds forever.
      const callsAtRest = getJob.mock.calls.length;
      await act(async () => {
        jest.advanceTimersByTime(30000);
      });
      expect(getJob.mock.calls.length).toBe(callsAtRest);
    } finally {
      jest.useRealTimers();
    }
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
