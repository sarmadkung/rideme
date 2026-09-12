import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ApiClient } from '@platform/api-client';
import type { GroceryOrder } from '@platform/types';
import { GroceryCheckoutScreen } from './GroceryCheckoutScreen';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

function aCart(): GroceryOrder {
  return {
    id: 'order-1',
    status: 'CART',
    items_total: { amount_minor: 250_00, currency: 'PKR' },
    items: [
      {
        id: 'line-1',
        name: 'Basmati rice 5kg',
        quantity: 1,
        unit_price: { amount_minor: 250_00, currency: 'PKR' },
        line_total: { amount_minor: 250_00, currency: 'PKR' },
        substitution_preference: 'ALLOW',
        status: 'ORDERED',
      },
    ],
    created_at: '2026-09-12T09:00:00Z',
  } as GroceryOrder;
}

function checkout(
  overrides: Partial<GroceryState & GroceryActions> = {},
): GroceryState & GroceryActions {
  return {
    stage: 'checkout',
    stores: [],
    store: null,
    products: [],
    order: aCart(),
    pending: false,
    error: null,
    findStores: jest.fn(async () => {}),
    enterStore: jest.fn(async () => {}),
    addItem: jest.fn(async () => {}),
    toCheckout: jest.fn(),
    backToShopping: jest.fn(),
    place: jest.fn(async () => {}),
    decide: jest.fn(async () => {}),
    cancel: jest.fn(async () => {}),
    reset: jest.fn(),
    ...overrides,
  };
}

const client = { searchPlaces: jest.fn(async () => []) } as never as ApiClient;

describe('GroceryCheckoutScreen', () => {
  it('will not place an order with nowhere to send it', () => {
    // `MarkReady` refuses an order with no destination, because a delivery job
    // needs both ends of a route. Asking here beats support chasing it later.
    const place = jest.fn(async () => {});
    render(<GroceryCheckoutScreen grocery={checkout({ place })} client={client} />);

    fireEvent.press(screen.getByTestId('checkout-place'));
    expect(place).not.toHaveBeenCalled();
  });

  it('sends the address the customer edited, not the one the geocoder guessed', () => {
    // No geocoder knows a flat number, and a rider has to find the door.
    const place = jest.fn(async () => {});
    render(<GroceryCheckoutScreen grocery={checkout({ place })} client={client} />);

    fireEvent.press(screen.getByTestId('checkout-address-Liberty Market'));
    fireEvent.changeText(screen.getByTestId('checkout-address-text'), 'House 12, Street 4');
    fireEvent.changeText(screen.getByTestId('checkout-notes'), 'Second gate');
    fireEvent.press(screen.getByTestId('checkout-place'));

    expect(place).toHaveBeenCalledWith({
      address: 'House 12, Street 4',
      latitude: 31.5169,
      longitude: 74.3484,
      notes: 'Second gate',
    });
  });

  it('sends no notes rather than an empty string', () => {
    const place = jest.fn(async () => {});
    render(<GroceryCheckoutScreen grocery={checkout({ place })} client={client} />);

    fireEvent.press(screen.getByTestId('checkout-address-Liberty Market'));
    fireEvent.press(screen.getByTestId('checkout-place'));

    expect(place).toHaveBeenCalledWith(expect.not.objectContaining({ notes: expect.anything() }));
  });

  it('shows the items total and promises no delivery price it does not have', () => {
    // Delivery is quoted when the order becomes a job. A number here would be
    // the client computing money.
    render(<GroceryCheckoutScreen grocery={checkout()} client={client} />);
    expect(screen.getByTestId('checkout-total')).toHaveTextContent('PKR 250.00');
    expect(screen.getByTestId('checkout-summary')).toHaveTextContent(/charged separately/);
  });
});
