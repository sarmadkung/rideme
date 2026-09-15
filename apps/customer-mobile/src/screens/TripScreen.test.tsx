import { fireEvent, render, screen } from '@testing-library/react-native';
import type { Job } from '@platform/types';
import { distanceKm, distanceText, TripScreen } from './TripScreen';
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

function tracking(job: Job, overrides: Partial<BookingState> = {}): BookingState & BookingActions {
  return {
    stage: 'tracking',
    pickup: null,
    dropoff: null,
    quote: null,
    job,
    cancellation: null,
    driverPosition: null,
    live: false,
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

describe("the driver's approach", () => {
  const pickup = { latitude: 31.5204, longitude: 74.3587 };
  // Roughly 1.2 km north of the pickup.
  const nearby = { latitude: 31.5312, longitude: 74.3587, recordedAt: '2026-09-15T09:00:00Z' };

  it('tells the customer how far away the driver is', () => {
    render(<TripScreen booking={tracking(aJob('ARRIVING'), { pickup, driverPosition: nearby })} />);
    expect(screen.getByTestId('trip-distance')).toHaveTextContent(
      'Your driver is about 1.2 km away',
    );
  });

  // Once the customer is in the vehicle, "your driver is 0.0 km away" is noise.
  it('says nothing once the trip has started', () => {
    render(
      <TripScreen booking={tracking(aJob('IN_PROGRESS'), { pickup, driverPosition: nearby })} />,
    );
    expect(screen.queryByTestId('trip-distance')).toBeNull();
  });

  // A distance that appears and vanishes as the stream reconnects would be
  // worse than none.
  it('says nothing before a position has arrived', () => {
    render(<TripScreen booking={tracking(aJob('ARRIVING'), { pickup })} />);
    expect(screen.queryByTestId('trip-distance')).toBeNull();
  });

  it('reads in metres up close and announces arrival', () => {
    expect(distanceText(0.02)).toBe('Your driver is here');
    expect(distanceText(0.32)).toContain('m away');
    expect(distanceText(4.25)).toContain('4.3 km away');
  });

  it('measures a known distance', () => {
    // One degree of latitude is about 111 km.
    const km = distanceKm({ latitude: 31, longitude: 74 }, { latitude: 32, longitude: 74 });
    expect(km).toBeGreaterThan(110);
    expect(km).toBeLessThan(112);
    expect(distanceKm(pickup, pickup)).toBeCloseTo(0);
  });
});
