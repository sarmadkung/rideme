import { fireEvent, render, screen } from '@testing-library/react-native';
import type { GroceryOrder, Product } from '@platform/types';
import { ShopScreen } from './ShopScreen';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

function aProduct(overrides: Partial<Product> = {}): Product {
  return {
    id: 'p-1',
    name: 'Basmati rice 5kg',
    price: { amount_minor: 250_00, currency: 'PKR' },
    available: true,
    ...overrides,
  } as Product;
}

function aCart(overrides: Partial<GroceryOrder> = {}): GroceryOrder {
  return {
    id: 'order-1',
    status: 'CART',
    items_total: { amount_minor: 0, currency: 'PKR' },
    items: [],
    created_at: '2026-09-12T09:00:00Z',
    ...overrides,
  } as GroceryOrder;
}

function shopping(
  overrides: Partial<GroceryState & GroceryActions> = {},
): GroceryState & GroceryActions {
  return {
    stage: 'shopping',
    stores: [],
    store: {
      id: 'store-1',
      merchant_name: 'Al-Fatah',
      name: 'Gulberg',
      latitude: 31.5,
      longitude: 74.3,
      distance_m: 400,
      open: true,
    },
    products: [aProduct()],
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

describe('ShopScreen', () => {
  it('adds a line with the quantity and preference chosen for it', () => {
    // Document 74's preference is per line and asked at the moment the line is
    // added, because that is the only moment the customer is thinking about
    // this particular item.
    const addItem = jest.fn(async () => {});
    render(<ShopScreen grocery={shopping({ addItem })} />);

    fireEvent.press(screen.getByTestId('product-p-1-more'));
    fireEvent.press(screen.getByTestId('product-p-1-DO_NOT_ALLOW'));
    fireEvent.press(screen.getByTestId('product-p-1-add'));

    expect(addItem).toHaveBeenCalledWith(expect.objectContaining({ id: 'p-1' }), 2, 'DO_NOT_ALLOW');
  });

  it('never asks for less than one of anything', () => {
    render(<ShopScreen grocery={shopping()} />);
    fireEvent.press(screen.getByTestId('product-p-1-less'));
    expect(screen.getByTestId('product-p-1-quantity')).toHaveTextContent('1');
  });

  it('shows an unavailable product without a way to add it', () => {
    const grocery = shopping({ products: [aProduct({ available: false })] });
    render(<ShopScreen grocery={grocery} />);
    expect(screen.queryByTestId('product-p-1-add')).toBeNull();
  });

  it('shows the total the server computed, not one it added up itself', () => {
    const cart = aCart({
      items_total: { amount_minor: 500_00, currency: 'PKR' },
      items: [
        {
          id: 'line-1',
          name: 'Basmati rice 5kg',
          quantity: 2,
          unit_price: { amount_minor: 250_00, currency: 'PKR' },
          line_total: { amount_minor: 500_00, currency: 'PKR' },
          substitution_preference: 'ALLOW',
          status: 'ORDERED',
        },
      ],
    });
    render(<ShopScreen grocery={shopping({ order: cart })} />);
    expect(screen.getByTestId('cart-total')).toHaveTextContent('PKR 500.00');
  });

  it('will not check out an empty cart', () => {
    // The server refuses it with "there is nothing in this cart yet". Refusing
    // it here saves the round trip and the moment of doubt.
    const toCheckout = jest.fn();
    render(<ShopScreen grocery={shopping({ toCheckout })} />);

    fireEvent.press(screen.getByTestId('shop-checkout'));
    expect(toCheckout).not.toHaveBeenCalled();
  });
});
