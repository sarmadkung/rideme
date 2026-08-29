import { fireEvent, render, screen } from '@testing-library/react-native';
import type { Job } from '@platform/types';
import { TripScreen } from './TripScreen';
import type { BookingActions, BookingState } from '../features/booking/useBooking';

function aJob(status: string, overrides: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    type: 'RIDE',
    status,
    stops: [],
    created_at: '2026-08-29T12:00:00Z',
    ...overrides,
  } as Job;
}

function tracking(
  job: Job,
  overrides: Partial<BookingState> = {},
): BookingState & BookingActions {
  return {
    stage: 'tracking',
    pickup: null,
    dropoff: null,
    quote: null,
    job,
    cancellation: null,
    pending: false,
    error: null,
    setPickup: jest.fn(),
    setDropoff: jest.fn(),
    requestQuote: jest.fn(async () => {}),
    confirm: jest.fn(async () => {}),
    cancel: jest.fn(async () => {}),
    reset: jest.fn(),
    ...overrides,
  };
}

describe('TripScreen', () => {
  it('names each stage of the ride', () => {
    render(<TripScreen booking={tracking(aJob('SEARCHING'))} />);
    expect(screen.getByTestId('trip-status')).toHaveTextContent('Finding a driver');
  });

  it('explains BD-04 expiry in plain words and says nothing was charged', () => {
    // "Expired" on its own reads like a payment problem. A customer who found
    // no driver needs to know they owe nothing.
    render(<TripScreen booking={tracking(aJob('EXPIRED'))} />);
    expect(screen.getByTestId('trip-status')).toHaveTextContent('No drivers available');
    expect(screen.getByText(/have not been charged/i)).toBeTruthy();
  });

  it('offers cancellation while the ride can still be cancelled', () => {
    const state = tracking(aJob('ACCEPTED'));
    render(<TripScreen booking={state} />);

    fireEvent.press(screen.getByTestId('cancel-ride'));
    expect(state.cancel).toHaveBeenCalled();
  });

  it('withdraws cancellation once the trip is under way', () => {
    // Document 036: after start, normal cancellation is not permitted.
    render(<TripScreen booking={tracking(aJob('IN_PROGRESS'))} />);
    expect(screen.queryByTestId('cancel-ride')).toBeNull();
  });

  it('tells the customer a cancellation was free', () => {
    render(
      <TripScreen
        booking={tracking(aJob('CANCELLED'), {
          cancellation: {
            job: aJob('CANCELLED'),
            cancellation_tier: 'BEFORE_ASSIGNMENT',
            fee: { amount_minor: 0, currency: 'PKR' },
          },
        })}
      />,
    );
    expect(screen.getByTestId('cancellation-summary')).toHaveTextContent(/not charged/);
  });

  it('states the fee when BD-01 charged one', () => {
    render(
      <TripScreen
        booking={tracking(aJob('CANCELLED'), {
          cancellation: {
            job: aJob('CANCELLED'),
            cancellation_tier: 'AFTER_ASSIGNMENT',
            fee: { amount_minor: 10000, currency: 'PKR' },
          },
        })}
      />,
    );
    expect(screen.getByTestId('cancellation-summary')).toHaveTextContent(/PKR 100\.00/);
  });

  it('offers a new ride once this one is over', () => {
    const state = tracking(aJob('COMPLETED'));
    render(<TripScreen booking={state} />);

    fireEvent.press(screen.getByTestId('book-another'));
    expect(state.reset).toHaveBeenCalled();
  });

  it('falls back to the raw status rather than showing nothing', () => {
    render(<TripScreen booking={tracking(aJob('SOMETHING_NEW'))} />);
    expect(screen.getByTestId('trip-status')).toHaveTextContent('SOMETHING_NEW');
  });
});
