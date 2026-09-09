import { fireEvent, render, screen } from '@testing-library/react-native';
import type { Quote } from '@platform/types';
import type { ApiClient, Place } from '@platform/api-client';
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

/** A client whose search finds nothing, so the landmark floor stays visible. */
function aClient(places: Place[] = []): ApiClient {
  return { searchPlaces: jest.fn(async () => places) } as unknown as ApiClient;
}

describe('BookingScreen', () => {
  it('asks for a price once both ends are chosen', () => {
    const [first, second] = PLACES;
    const state = booking({ pickup: first?.stop ?? null, dropoff: second?.stop ?? null });
    render(<BookingScreen booking={state} client={aClient()} />);

    fireEvent.press(screen.getByTestId('get-quote'));
    expect(state.requestQuote).toHaveBeenCalled();
  });

  it('shows every fare line, not only the total', () => {
    // Document 034 requires the breakdown, and BD-02's demand line can move a
    // fare — a customer charged extra must be able to see which line did it.
    render(
      <BookingScreen booking={booking({ stage: 'quoted', quote: aQuote() })} client={aClient()} />,
    );

    expect(screen.getByText('Base fare')).toBeTruthy();
    expect(screen.getByText('Distance')).toBeTruthy();
    expect(screen.getByText('Busy area')).toBeTruthy();
    expect(screen.getByTestId('quote-total')).toHaveTextContent('PKR 450.00');
  });

  it('says so when the route is only an estimate', () => {
    // Document 096 forbids presenting a fallback as exact.
    render(
      <BookingScreen booking={booking({ stage: 'quoted', quote: aQuote() })} client={aClient()} />,
    );
    expect(screen.getByTestId('route-disclaimer')).toBeTruthy();
  });

  it('drops the disclaimer for a measured route', () => {
    render(
      <BookingScreen
        booking={booking({ stage: 'quoted', quote: aQuote({ route_confidence: 'MEASURED' }) })}
        client={aClient()}
      />,
    );
    expect(screen.queryByTestId('route-disclaimer')).toBeNull();
  });

  it('surfaces an error from the server', () => {
    render(
      <BookingScreen
        booking={booking({ error: 'this service is not available here yet' })}
        client={aClient()}
      />,
    );
    expect(screen.getByTestId('booking-error')).toHaveTextContent(
      'this service is not available here yet',
    );
  });

  it('does not offer confirm before there is a price', () => {
    render(<BookingScreen booking={booking()} client={aClient()} />);
    expect(screen.queryByTestId('confirm-booking')).toBeNull();
  });
});

describe('BookingScreen place search', () => {
  function aFoundPlace(name: string): Place {
    return { name, address: `${name}, Gulberg III, Lahore`, latitude: 31.5169, longitude: 74.3484 };
  }

  it('books somewhere that is not one of the five landmarks', async () => {
    // The whole point: before this, a customer could only choose from a fixed
    // list, and everywhere else in the city was unbookable.
    const state = booking();
    const client = aClient([aFoundPlace('Packages Mall')]);
    render(<BookingScreen booking={state} client={client} />);

    fireEvent.changeText(screen.getByTestId('pickup-picker-input'), 'Packages');
    const result = await screen.findByTestId('pickup-picker-result-0');
    fireEvent.press(result);

    expect(state.setPickup).toHaveBeenCalledWith({ latitude: 31.5169, longitude: 74.3484 });
  });

  it('shows the address as well as the name', async () => {
    // The name is what a customer recognises; the address is how they tell two
    // places with the same name apart.
    render(<BookingScreen booking={booking()} client={aClient([aFoundPlace('Liberty Market')])} />);

    fireEvent.changeText(screen.getByTestId('dropoff-picker-input'), 'Liberty');
    expect(await screen.findByText(/Gulberg III, Lahore/)).toBeTruthy();
  });

  it('keeps the landmarks reachable when search finds nothing', async () => {
    // Search can be unavailable and a customer must still be able to book.
    render(<BookingScreen booking={booking()} client={aClient([])} />);

    const landmark = PLACES[0];
    expect(landmark).toBeDefined();
    expect(screen.getByTestId(`pickup-picker-${landmark?.name}`)).toBeTruthy();
  });

  it('says an outage is an outage rather than showing no results', async () => {
    const failing = {
      searchPlaces: jest.fn(async () => {
        throw new Error('unreachable');
      }),
    } as unknown as ApiClient;
    render(<BookingScreen booking={booking()} client={failing} />);

    fireEvent.changeText(screen.getByTestId('pickup-picker-input'), 'Liberty');

    expect(await screen.findByTestId('pickup-picker-search-error')).toBeTruthy();
    expect(screen.queryByTestId('pickup-picker-no-results')).toBeNull();
  });

  it('collapses to the chosen place, with a way back', () => {
    // Once a stop is chosen the search box is not the useful control any more;
    // changing your mind is.
    const state = booking({ pickup: PLACES[0]?.stop ?? null });
    render(<BookingScreen booking={state} client={aClient()} />);

    expect(screen.queryByTestId('pickup-picker-input')).toBeNull();
    fireEvent.press(screen.getByTestId('pickup-picker-clear'));
    expect(state.setPickup).toHaveBeenCalledWith(null);
  });
});
