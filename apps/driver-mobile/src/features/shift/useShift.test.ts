import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError, type ApiClient } from '@platform/api-client';
import type { DriverAssignment, DriverProfile, Job } from '@platform/types';
import { isOffer, isOnline, nextCommand, useShift } from './useShift';

function aDriver(overrides: Partial<DriverProfile> = {}): DriverProfile {
  return {
    id: 'drv-1',
    status: 'AVAILABLE',
    active_vehicle_id: 'veh-1',
    verification_status: 'APPROVED',
    ...overrides,
  };
}

function aJob(status: string, overrides: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    type: 'RIDE',
    status,
    stops: [
      { id: 's1', sequence: 0, type: 'PICKUP', latitude: 31.5204, longitude: 74.3587 },
      { id: 's2', sequence: 1, type: 'DROPOFF', latitude: 31.588, longitude: 74.315 },
    ],
    created_at: '2026-08-29T12:00:00Z',
    ...overrides,
  } as Job;
}

function anOffer(status = 'OFFERED', job = aJob('SEARCHING')): DriverAssignment {
  return {
    id: 'asg-1',
    status,
    offered_at: '2026-08-29T12:00:00Z',
    expires_at: '2026-08-29T12:00:20Z',
    job,
  };
}

function stubClient(overrides: Record<string, unknown> = {}) {
  return {
    driverMe: jest.fn(async () => aDriver()),
    driverAssignment: jest.fn(async () => null),
    goOnline: jest.fn(async () => aDriver({ status: 'AVAILABLE' })),
    goOffline: jest.fn(async () => aDriver({ status: 'OFFLINE' })),
    driverAccept: jest.fn(async () => aJob('ACCEPTED')),
    driverReject: jest.fn(async () => aJob('SEARCHING')),
    driverArrive: jest.fn(async () => aJob('AT_PICKUP')),
    driverStart: jest.fn(async () => aJob('IN_PROGRESS')),
    driverComplete: jest.fn(async () => aJob('COMPLETED')),
    reportLocation: jest.fn(async () => ({ accepted: 1, rejected: [] })),
    ...overrides,
  };
}

function asClient(stub: ReturnType<typeof stubClient>): ApiClient {
  return stub as unknown as ApiClient;
}

const HERE = { latitude: 31.5204, longitude: 74.3587 };

describe('useShift', () => {
  it('reads the driver and their assignment together', async () => {
    // Reading only one leaves the app showing an offer for a driver who has
    // gone offline, or an online driver with no sign of the job they hold.
    const client = stubClient({ driverAssignment: jest.fn(async () => anOffer()) });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.driver?.id).toBe('drv-1');
    expect(result.current.assignment?.id).toBe('asg-1');
  });

  it('goes online and offline', async () => {
    const client = stubClient();
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.goOnline(HERE);
    });
    expect(client.goOnline).toHaveBeenCalledWith(HERE);
    expect(result.current.driver?.status).toBe('AVAILABLE');

    await act(async () => {
      await result.current.goOffline();
    });
    expect(result.current.driver?.status).toBe('OFFLINE');
  });

  it('drops the held assignment when the driver signs off', async () => {
    // Keeping it would offer commands the server will refuse.
    const client = stubClient({ driverAssignment: jest.fn(async () => anOffer()) });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.assignment).not.toBeNull();

    await act(async () => {
      await result.current.goOffline();
    });
    expect(result.current.assignment).toBeNull();
  });

  it('re-reads from the server after accepting rather than guessing', async () => {
    // Patching the job locally drifts the moment a command partly succeeds.
    const client = stubClient({
      driverAssignment: jest
        .fn()
        .mockResolvedValueOnce(anOffer())
        .mockResolvedValue(anOffer('ACCEPTED', aJob('ACCEPTED'))),
    });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.refresh();
    });
    await act(async () => {
      await result.current.accept();
    });

    expect(client.driverAccept).toHaveBeenCalledWith('job-1');
    expect(result.current.assignment?.job.status).toBe('ACCEPTED');
  });

  it('surfaces a refused command', async () => {
    const client = stubClient({
      driverAssignment: jest.fn(async () => anOffer()),
      driverAccept: jest.fn(async () => {
        throw new ApiError(409, {
          code: 'conflict',
          message: 'this offer has expired',
          request_id: 'r',
        });
      }),
    });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.refresh();
    });
    await act(async () => {
      await result.current.accept();
    });

    expect(result.current.error).toBe('this offer has expired');
  });

  it('sends the one command the trip is up to', async () => {
    const client = stubClient({
      driverAssignment: jest.fn(async () => anOffer('ACCEPTED', aJob('AT_PICKUP'))),
    });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.refresh();
    });
    await act(async () => {
      await result.current.advance();
    });

    expect(client.driverStart).toHaveBeenCalledWith('job-1');
    expect(client.driverArrive).not.toHaveBeenCalled();
    expect(client.driverComplete).not.toHaveBeenCalled();
  });

  it('swallows a failed position report', async () => {
    // The next one is seconds away. A banner that flickers on every dropped
    // request teaches drivers to ignore banners.
    const client = stubClient({
      reportLocation: jest.fn(async () => {
        throw new Error('offline');
      }),
    });
    const { result } = renderHook(() => useShift(asClient(client), { pollMs: 100000 }));

    await act(async () => {
      await result.current.report(HERE);
    });
    expect(result.current.error).toBeNull();
  });

  it('counts an offer down from the server expiry', async () => {
    // A local TTL would disagree with the server and let a driver tap Accept
    // on an offer that has already gone.
    const at = new Date('2026-08-29T12:00:05Z').getTime();
    const client = stubClient({ driverAssignment: jest.fn(async () => anOffer()) });
    const { result } = renderHook(() =>
      useShift(asClient(client), { pollMs: 100000, now: () => at }),
    );

    await act(async () => {
      await result.current.refresh();
    });
    await waitFor(() => expect(result.current.offerSecondsLeft).toBe(15));
  });

  it('stops polling once the driver is offline', async () => {
    jest.useFakeTimers();
    try {
      const client = stubClient({ driverMe: jest.fn(async () => aDriver({ status: 'OFFLINE' })) });
      const { result } = renderHook(() => useShift(asClient(client), { pollMs: 1000 }));

      await act(async () => {
        await result.current.refresh();
      });
      const callsAtRest = client.driverMe.mock.calls.length;

      await act(async () => {
        jest.advanceTimersByTime(10000);
      });
      // An offline driver is not waiting for anything, and polling would spend
      // their battery on it.
      expect(client.driverMe.mock.calls.length).toBe(callsAtRest);
    } finally {
      jest.useRealTimers();
    }
  });
});

describe('nextCommand', () => {
  it('walks document 035 one step at a time', () => {
    expect(nextCommand(aJob('ACCEPTED'))?.action).toBe('arrive');
    expect(nextCommand(aJob('ARRIVING'))?.action).toBe('arrive');
    expect(nextCommand(aJob('AT_PICKUP'))?.action).toBe('start');
    expect(nextCommand(aJob('IN_PROGRESS'))?.action).toBe('complete');
  });

  it('offers nothing for a job that is not the driver’s to move', () => {
    expect(nextCommand(aJob('SEARCHING'))).toBeNull();
    expect(nextCommand(aJob('COMPLETED'))).toBeNull();
    expect(nextCommand(aJob('CANCELLED'))).toBeNull();
    expect(nextCommand(null)).toBeNull();
  });
});

describe('isOnline', () => {
  it('treats suspension as not working', () => {
    expect(isOnline(aDriver({ status: 'AVAILABLE' }))).toBe(true);
    expect(isOnline(aDriver({ status: 'ON_TRIP' }))).toBe(true);
    expect(isOnline(aDriver({ status: 'OFFLINE' }))).toBe(false);
    expect(isOnline(aDriver({ status: 'SUSPENDED' }))).toBe(false);
    expect(isOnline(null)).toBe(false);
  });
});

describe('isOffer', () => {
  it('separates an unanswered offer from a trip under way', () => {
    expect(isOffer(anOffer('OFFERED'))).toBe(true);
    expect(isOffer(anOffer('ACCEPTED'))).toBe(false);
    expect(isOffer(null)).toBe(false);
  });
});
