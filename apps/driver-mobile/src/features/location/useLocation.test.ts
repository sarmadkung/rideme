import { act, renderHook, waitFor } from '@testing-library/react-native';
import * as Location from 'expo-location';
import { toPositionInput, useLocation } from './useLocation';

jest.mock('expo-location', () => ({
  Accuracy: { High: 4 },
  hasServicesEnabledAsync: jest.fn(),
  requestForegroundPermissionsAsync: jest.fn(),
  watchPositionAsync: jest.fn(),
}));

const mocked = Location as jest.Mocked<typeof Location>;

/** One platform fix. Coordinates are Gulberg, Lahore. */
function aFix(
  overrides: Partial<Location.LocationObjectCoords> = {},
  timestamp = 1_756_000_000_000,
) {
  return {
    timestamp,
    coords: {
      latitude: 31.5204,
      longitude: 74.3587,
      accuracy: 12,
      heading: 90,
      speed: 8,
      altitude: 210,
      altitudeAccuracy: 5,
      ...overrides,
    },
  } as Location.LocationObject;
}

/** Captures the callback so a test can deliver fixes on its own schedule. */
function captureWatcher() {
  const remove = jest.fn();
  let deliver: ((fix: Location.LocationObject) => void) | null = null;
  mocked.watchPositionAsync.mockImplementation(async (_options, callback) => {
    deliver = callback;
    return { remove } as unknown as Location.LocationSubscription;
  });
  return {
    remove,
    deliver: (fix: Location.LocationObject) => {
      if (deliver === null) throw new Error('watchPositionAsync was never called');
      act(() => deliver!(fix));
    },
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mocked.hasServicesEnabledAsync.mockResolvedValue(true);
  mocked.requestForegroundPermissionsAsync.mockResolvedValue({ granted: true } as never);
});

describe('toPositionInput', () => {
  it('carries the four fields document 048 needs to judge a fix', () => {
    const position = toPositionInput(aFix());
    expect(position).toMatchObject({
      latitude: 31.5204,
      longitude: 74.3587,
      accuracyM: 12,
      headingDeg: 90,
      speedMps: 8,
    });
    // Timed by the device, not by arrival.
    expect(position.recordedAt).toEqual(new Date(1_756_000_000_000));
  });

  it('omits heading and speed rather than reporting the sentinel as a value', () => {
    // A stationary device reports -1 on some Android hardware. Sending it would
    // claim the driver faces a direction that does not exist.
    const position = toPositionInput(aFix({ heading: -1, speed: -1, accuracy: null }));
    expect(position).not.toHaveProperty('headingDeg');
    expect(position).not.toHaveProperty('speedMps');
    expect(position).not.toHaveProperty('accuracyM');
    expect(position.latitude).toBe(31.5204);
  });
});

describe('useLocation', () => {
  it('starts with no position rather than a guess', async () => {
    captureWatcher();
    const { result } = renderHook(() => useLocation());
    expect(result.current.position).toBeNull();
    expect(result.current.stage).toBe('starting');
    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());
  });

  it('reports a fix once the platform delivers one', async () => {
    const watcher = captureWatcher();
    const { result } = renderHook(() => useLocation());
    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());

    watcher.deliver(aFix());

    await waitFor(() => expect(result.current.stage).toBe('tracking'));
    expect(result.current.position?.latitude).toBe(31.5204);
    expect(result.current.message).toBeNull();
  });

  it('says the driver refused, and does not watch', async () => {
    mocked.requestForegroundPermissionsAsync.mockResolvedValue({ granted: false } as never);
    const { result } = renderHook(() => useLocation());

    await waitFor(() => expect(result.current.stage).toBe('denied'));
    expect(result.current.position).toBeNull();
    expect(result.current.message).toContain('Settings');
    expect(mocked.watchPositionAsync).not.toHaveBeenCalled();
  });

  it('distinguishes services being off from permission being refused', async () => {
    // They need different actions from the driver, so one message for both
    // would send half of them to the wrong screen.
    mocked.hasServicesEnabledAsync.mockResolvedValue(false);
    const { result } = renderHook(() => useLocation());

    await waitFor(() => expect(result.current.stage).toBe('disabled'));
    expect(mocked.requestForegroundPermissionsAsync).not.toHaveBeenCalled();
  });

  it('recovers when the driver grants permission and retries', async () => {
    // A refusal is not permanent; the app must not need a restart to notice.
    mocked.requestForegroundPermissionsAsync.mockResolvedValue({ granted: false } as never);
    const { result } = renderHook(() => useLocation());
    await waitFor(() => expect(result.current.stage).toBe('denied'));

    const watcher = captureWatcher();
    mocked.requestForegroundPermissionsAsync.mockResolvedValue({ granted: true } as never);
    act(() => result.current.retry());

    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());
    watcher.deliver(aFix());
    await waitFor(() => expect(result.current.stage).toBe('tracking'));
  });

  it('keeps the better fix when a rough one arrives after it', async () => {
    const watcher = captureWatcher();
    const { result } = renderHook(() => useLocation());
    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());

    watcher.deliver(aFix({ accuracy: 10 }));
    await waitFor(() => expect(result.current.position?.accuracyM).toBe(10));

    watcher.deliver(aFix({ latitude: 31.9, accuracy: 900 }));

    // Still the accurate one: moving the driver to a point the device is not
    // confident about would offer them jobs across the city.
    expect(result.current.position?.accuracyM).toBe(10);
    expect(result.current.position?.latitude).toBe(31.5204);
  });

  it('accepts a rough first fix rather than stranding a driver indoors', async () => {
    const watcher = captureWatcher();
    const { result } = renderHook(() => useLocation());
    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());

    watcher.deliver(aFix({ accuracy: 900 }));

    await waitFor(() => expect(result.current.stage).toBe('tracking'));
    expect(result.current.position?.accuracyM).toBe(900);
  });

  it('surfaces an unclassifiable platform failure instead of waiting forever', async () => {
    mocked.hasServicesEnabledAsync.mockRejectedValue(new Error('no provider'));
    const { result } = renderHook(() => useLocation());

    await waitFor(() => expect(result.current.stage).toBe('unavailable'));
    expect(result.current.position).toBeNull();
  });

  it('stops watching when the screen goes away', async () => {
    const watcher = captureWatcher();
    const { unmount } = renderHook(() => useLocation());
    await waitFor(() => expect(mocked.watchPositionAsync).toHaveBeenCalled());

    unmount();

    // A GPS session left running is the battery cost the whole design exists
    // to avoid.
    await waitFor(() => expect(watcher.remove).toHaveBeenCalled());
  });
});
