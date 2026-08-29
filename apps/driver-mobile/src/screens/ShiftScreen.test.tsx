import { fireEvent, render, screen } from '@testing-library/react-native';
import type { DriverProfile } from '@platform/types';
import { ShiftScreen } from './ShiftScreen';
import type { ShiftActions, ShiftState } from '../features/shift/useShift';

const HERE = { latitude: 31.5204, longitude: 74.3587 };

function aDriver(overrides: Partial<DriverProfile> = {}): DriverProfile {
  return {
    id: 'drv-1',
    status: 'OFFLINE',
    active_vehicle_id: 'veh-1',
    verification_status: 'APPROVED',
    ...overrides,
  };
}

function shift(driver: DriverProfile | null): ShiftState & ShiftActions {
  return {
    driver,
    assignment: null,
    pending: false,
    error: null,
    offerSecondsLeft: null,
    refresh: jest.fn(async () => {}),
    goOnline: jest.fn(async () => {}),
    goOffline: jest.fn(async () => {}),
    accept: jest.fn(async () => {}),
    reject: jest.fn(async () => {}),
    advance: jest.fn(async () => {}),
    report: jest.fn(async () => {}),
  };
}

describe('ShiftScreen', () => {
  it('goes online and offline', () => {
    const offline = shift(aDriver({ status: 'OFFLINE' }));
    render(<ShiftScreen shift={offline} position={HERE} />);
    fireEvent.press(screen.getByTestId('shift-toggle'));
    expect(offline.goOnline).toHaveBeenCalledWith(HERE);

    const online = shift(aDriver({ status: 'AVAILABLE' }));
    render(<ShiftScreen shift={online} position={HERE} />);
    fireEvent.press(screen.getAllByTestId('shift-toggle')[1]!);
    expect(online.goOffline).toHaveBeenCalled();
  });

  it('says why a driver cannot go online rather than doing nothing', () => {
    // "Go online" that silently fails is the worst version of this screen.
    const unverified = shift(aDriver({ verification_status: 'UNDER_REVIEW' }));
    render(<ShiftScreen shift={unverified} position={HERE} />);
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/being verified/i);

    fireEvent.press(screen.getByTestId('shift-toggle'));
    expect(unverified.goOnline).not.toHaveBeenCalled();
  });

  it('names the missing vehicle specifically', () => {
    // Dispatch matches jobs to vehicle capabilities, so a driver with none is
    // never offered anything and would sit online wondering why.
    const state = shift(aDriver({ active_vehicle_id: undefined }));
    render(<ShiftScreen shift={state} position={HERE} />);
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/active vehicle/i);
  });

  it('waits for a location before letting a driver go online', () => {
    const state = shift(aDriver());
    render(<ShiftScreen shift={state} position={null} />);
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/location/i);

    fireEvent.press(screen.getByTestId('shift-toggle'));
    expect(state.goOnline).not.toHaveBeenCalled();
  });
});
