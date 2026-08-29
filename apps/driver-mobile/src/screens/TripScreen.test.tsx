import { fireEvent, render, screen } from '@testing-library/react-native';
import type { DriverAssignment, Job } from '@platform/types';
import { TripScreen } from './TripScreen';
import type { ShiftActions, ShiftState } from '../features/shift/useShift';

function aJob(status: string): Job {
  return {
    id: 'job-1',
    type: 'RIDE',
    status,
    stops: [
      {
        id: 's1',
        sequence: 0,
        type: 'PICKUP',
        latitude: 31.5204,
        longitude: 74.3587,
        address: 'Liberty Market',
      },
      {
        id: 's2',
        sequence: 1,
        type: 'DROPOFF',
        latitude: 31.588,
        longitude: 74.315,
        address: 'Anarkali Bazaar',
      },
    ],
    created_at: '2026-08-29T12:00:00Z',
  } as Job;
}

function shift(status: string, overrides: Partial<ShiftState> = {}): ShiftState & ShiftActions {
  const assignment: DriverAssignment = {
    id: 'asg-1',
    status: 'ACCEPTED',
    offered_at: '2026-08-29T12:00:00Z',
    job: aJob(status),
  };
  return {
    driver: null,
    assignment,
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
    ...overrides,
  };
}

describe('TripScreen', () => {
  it('offers exactly one command, the one the trip is up to', () => {
    // Showing every command at once would let a driver complete a trip they
    // never started.
    render(<TripScreen shift={shift('ACCEPTED')} />);
    expect(screen.getByTestId('trip-advance')).toHaveTextContent(/arrived/i);

    render(<TripScreen shift={shift('AT_PICKUP')} />);
    expect(screen.getAllByTestId('trip-advance')[1]).toHaveTextContent(/start trip/i);
  });

  it('points at the pickup before it, and the destination after', () => {
    render(<TripScreen shift={shift('ACCEPTED')} />);
    expect(screen.getByTestId('trip-heading')).toHaveTextContent(/Liberty Market/);

    render(<TripScreen shift={shift('IN_PROGRESS')} />);
    expect(screen.getAllByTestId('trip-heading')[1]).toHaveTextContent(/Anarkali Bazaar/);
  });

  it('sends the command', () => {
    const state = shift('IN_PROGRESS');
    render(<TripScreen shift={state} />);

    fireEvent.press(screen.getByTestId('trip-advance'));
    expect(state.advance).toHaveBeenCalled();
  });

  it('offers nothing once the job is out of the driver’s hands', () => {
    render(<TripScreen shift={shift('CANCELLED')} />);
    expect(screen.queryByTestId('trip-advance')).toBeNull();
    expect(screen.getByTestId('trip-no-command')).toBeTruthy();
  });
});
