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
  it.each([
    ['ACCEPTED', /arrived/i],
    ['ARRIVING', /arrived/i],
    ['AT_PICKUP', /start trip/i],
    ['IN_PROGRESS', /complete trip/i],
  ])('offers exactly the command %s is up to', (status, label) => {
    // Showing every command at once would let a driver complete a trip they
    // never started.
    const view = render(<TripScreen shift={shift(status)} />);
    expect(view.getByTestId('trip-advance')).toHaveTextContent(label);
    view.unmount();
  });

  it.each([
    ['ACCEPTED', /Liberty Market/],
    ['AT_PICKUP', /Liberty Market/],
    ['IN_PROGRESS', /Anarkali Bazaar/],
    ['AT_DROPOFF', /Anarkali Bazaar/],
  ])('points %s at the stop the driver is heading for', (status, expected) => {
    const view = render(<TripScreen shift={shift(status)} />);
    expect(view.getByTestId('trip-heading')).toHaveTextContent(expected);
    view.unmount();
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
