import { fireEvent, render, screen } from '@testing-library/react-native';
import type { DriverAssignment, Job } from '@platform/types';
import { OfferScreen } from './OfferScreen';
import type { ShiftActions, ShiftState } from '../features/shift/useShift';

function aJob(): Job {
  return {
    id: 'job-1',
    type: 'RIDE',
    status: 'SEARCHING',
    stops: [
      { id: 's1', sequence: 0, type: 'PICKUP', latitude: 31.5204, longitude: 74.3587 },
      { id: 's2', sequence: 1, type: 'DROPOFF', latitude: 31.588, longitude: 74.315 },
    ],
    created_at: '2026-08-29T12:00:00Z',
  } as Job;
}

function shift(overrides: Partial<ShiftState> = {}): ShiftState & ShiftActions {
  const assignment: DriverAssignment = {
    id: 'asg-1',
    status: 'OFFERED',
    offered_at: '2026-08-29T12:00:00Z',
    expires_at: '2026-08-29T12:00:20Z',
    job: aJob(),
  };
  return {
    driver: null,
    assignment,
    pending: false,
    error: null,
    offerSecondsLeft: 12,
    refresh: jest.fn(async () => {}),
    goOnline: jest.fn(async () => {}),
    goOffline: jest.fn(async () => {}),
    accept: jest.fn(async () => {}),
    reject: jest.fn(async () => {}),
    advance: jest.fn(async () => {}),
    report: jest.fn(async () => {}),
    ...overrides,
  };
}

describe('OfferScreen', () => {
  it('shows both ends of the trip and the countdown', () => {
    render(<OfferScreen shift={shift()} />);
    expect(screen.getByTestId('offer-countdown')).toHaveTextContent(/12s/);
    expect(screen.getByTestId('offer-pickup')).toBeTruthy();
    expect(screen.getByTestId('offer-dropoff')).toBeTruthy();
  });

  it('accepts and declines', () => {
    const state = shift();
    render(<OfferScreen shift={state} />);

    fireEvent.press(screen.getByTestId('offer-accept'));
    expect(state.accept).toHaveBeenCalled();

    fireEvent.press(screen.getByTestId('offer-reject'));
    expect(state.reject).toHaveBeenCalled();
  });

  it('stops accepting once the offer has lapsed', () => {
    // Tapping Accept on a lapsed offer would fail, and a button that always
    // fails is worse than no button.
    const state = shift({ offerSecondsLeft: 0 });
    render(<OfferScreen shift={state} />);

    expect(screen.getByTestId('offer-countdown')).toHaveTextContent(/expired/i);
    fireEvent.press(screen.getByTestId('offer-accept'));
    expect(state.accept).not.toHaveBeenCalled();
  });
});
