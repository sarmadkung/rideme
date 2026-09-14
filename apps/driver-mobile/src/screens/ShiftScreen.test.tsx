import { fireEvent, render, screen } from '@testing-library/react-native';
import type { DriverProfile } from '@platform/types';
import { ShiftScreen } from './ShiftScreen';
import type { ShiftActions, ShiftState } from '../features/shift/useShift';
import type { LocationActions, LocationState } from '../features/location/useLocation';

const HERE = { latitude: 31.5204, longitude: 74.3587 };

/** A hook that has a fix. */
function located(): LocationState & LocationActions {
  return { position: HERE, stage: 'tracking', message: null, retry: jest.fn() };
}

/** A hook that has none, for the reason given. */
function unlocated(
  stage: LocationState['stage'],
  message: string,
): LocationState & LocationActions {
  return { position: null, stage, message, retry: jest.fn() };
}

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
  it('goes online from offline', () => {
    const state = shift(aDriver({ status: 'OFFLINE' }));
    const view = render(
      <ShiftScreen shift={state} location={located()} onShowEarnings={jest.fn()} />,
    );

    fireEvent.press(view.getByTestId('shift-toggle'));
    expect(state.goOnline).toHaveBeenCalledWith(HERE);
    view.unmount();
  });

  it('goes offline from online', () => {
    const state = shift(aDriver({ status: 'AVAILABLE' }));
    const view = render(
      <ShiftScreen shift={state} location={located()} onShowEarnings={jest.fn()} />,
    );

    fireEvent.press(view.getByTestId('shift-toggle'));
    expect(state.goOffline).toHaveBeenCalled();
    view.unmount();
  });

  it('says why a driver cannot go online rather than doing nothing', () => {
    // "Go online" that silently fails is the worst version of this screen.
    const unverified = shift(aDriver({ verification_status: 'UNDER_REVIEW' }));
    render(<ShiftScreen shift={unverified} location={located()} onShowEarnings={jest.fn()} />);
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/being verified/i);

    fireEvent.press(screen.getByTestId('shift-toggle'));
    expect(unverified.goOnline).not.toHaveBeenCalled();
  });

  it('names the missing vehicle specifically', () => {
    // Dispatch matches jobs to vehicle capabilities, so a driver with none is
    // never offered anything and would sit online wondering why.
    const state = shift(aDriver({ active_vehicle_id: undefined }));
    render(<ShiftScreen shift={state} location={located()} onShowEarnings={jest.fn()} />);
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/active vehicle/i);
  });

  it('waits for a location before letting a driver go online', () => {
    const state = shift(aDriver());
    render(
      <ShiftScreen
        shift={state}
        location={unlocated('starting', 'Waiting for your location.')}
        onShowEarnings={jest.fn()}
      />,
    );
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/location/i);

    fireEvent.press(screen.getByTestId('shift-toggle'));
    expect(state.goOnline).not.toHaveBeenCalled();
  });

  it('passes the location hook s own reason through rather than flattening it', () => {
    // Refused and switched-off need different actions from the driver. One
    // sentence covering both sends half of them to the wrong setting.
    const state = shift(aDriver());
    render(
      <ShiftScreen
        shift={state}
        location={unlocated('denied', 'Enable it in Settings.')}
        onShowEarnings={jest.fn()}
      />,
    );
    expect(screen.getByTestId('shift-blocker')).toHaveTextContent(/Settings/);
  });

  it('offers a way back once a refusal is recoverable', () => {
    // A driver who grants permission in Settings must not have to restart the
    // app for it to be noticed.
    const state = shift(aDriver());
    const location = unlocated('denied', 'Enable it in Settings.');
    render(<ShiftScreen shift={state} location={location} onShowEarnings={jest.fn()} />);

    fireEvent.press(screen.getByTestId('location-retry'));
    expect(location.retry).toHaveBeenCalled();
  });

  it('does not offer a retry while the first fix is still coming', () => {
    // Nothing has failed yet; a button implying otherwise invites a driver to
    // fix something that is not broken.
    const state = shift(aDriver());
    render(
      <ShiftScreen
        shift={state}
        location={unlocated('starting', 'Waiting.')}
        onShowEarnings={jest.fn()}
      />,
    );
    expect(screen.queryByTestId('location-retry')).toBeNull();
  });

  it('offers a way to see earnings from the idle screen', () => {
    const onShowEarnings = jest.fn();
    render(
      <ShiftScreen shift={shift(aDriver())} location={located()} onShowEarnings={onShowEarnings} />,
    );

    fireEvent.press(screen.getByTestId('show-earnings'));
    expect(onShowEarnings).toHaveBeenCalled();
  });
});
