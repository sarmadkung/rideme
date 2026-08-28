import { fireEvent, render, screen } from '@testing-library/react-native';
import type { Quote } from '@platform/types';
import { BookingScreen, PLACES } from './BookingScreen';
import type { BookingActions, BookingState } from '../features/booking/useBooking';

function aQuote(overrides: Partial<Quote> = {}): Quote {
  return {
    quote_id: 'q-1',
    total: { amount_minor: 45000, currency: 'PKR' },
    lines: [
      { component: 'base', amount: { amount_minor: 5000, currency: 'PKR' } },
      { component: 'distance', amount: { amount_minor: 30000, currency: 'PKR' } },
      { component: 'demand', amount: { amount_minor: 10000, currency: 'PKR' } },
    ],
    distance_meters: 8200,
    duration_seconds: 1080,
    route_confidence: 'ESTIMATED',
    expires_at: '2026-08-29T12:05:00Z',
    ...overrides,
  } as Quote;
}

function booking(overrides: Partial<BookingState> = {}): BookingState & BookingActions {
  return {
    stage: 'planning',
    pickup: null,
    dropoff: null,
    quote: null,
    job: null,
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

describe('BookingScreen', () => {
  it('asks for a price once both ends are chosen', () => {
    const [first, second] = PLACES;
    const state = booking({ pickup: first?.stop ?? null, dropoff: second?.stop ?? null });
    render(<BookingScreen booking={state} />);

    fireEvent.press(screen.getByTestId('get-quote'));
    expect(state.requestQuote).toHaveBeenCalled();
  });

  it('shows every fare line, not only the total', () => {
    // Document 034 requires the breakdown, and BD-02's demand line can move a
    // fare — a customer charged extra must be able to see which line did it.
    render(<BookingScreen booking={booking({ stage: 'quoted', quote: aQuote() })} />);

    expect(screen.getByText('Base fare')).toBeTruthy();
    expect(screen.getByText('Distance')).toBeTruthy();
    expect(screen.getByText('Busy area')).toBeTruthy();
    expect(screen.getByTestId('quote-total')).toHaveTextContent('PKR 450.00');
  });

  it('says so when the route is only an estimate', () => {
    // Document 096 forbids presenting a fallback as exact.
    render(<BookingScreen booking={booking({ stage: 'quoted', quote: aQuote() })} />);
    expect(screen.getByTestId('route-disclaimer')).toBeTruthy();
  });

  it('drops the disclaimer for a measured route', () => {
    render(
      <BookingScreen
        booking={booking({ stage: 'quoted', quote: aQuote({ route_confidence: 'MEASURED' }) })}
      />,
    );
    expect(screen.queryByTestId('route-disclaimer')).toBeNull();
  });

  it('surfaces an error from the server', () => {
    render(<BookingScreen booking={booking({ error: 'this service is not available here yet' })} />);
    expect(screen.getByTestId('booking-error')).toHaveTextContent(
      'this service is not available here yet',
    );
  });

  it('does not offer confirm before there is a price', () => {
    render(<BookingScreen booking={booking()} />);
    expect(screen.queryByTestId('confirm-booking')).toBeNull();
  });
});
