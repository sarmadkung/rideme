import { fireEvent, render, screen } from '@testing-library/react-native';
import type { GroceryOrder, Product, ProductVariant } from '@platform/types';
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

function aVariant(overrides: Partial<ProductVariant> = {}): ProductVariant {
  return {
    id: 'v-1',
    name: '1L',
    price_diff: { amount_minor: 0, currency: 'PKR' },
    available: true,
    ...overrides,
  } as ProductVariant;
}

function withSizes(): Product {
  // Document 68's example: one product, two sizes, the larger carrying a price
  // difference rather than a price of its own.
  return aProduct({
    name: "Olper's milk",
    price: { amount_minor: 250_00, currency: 'PKR' },
    variants: [
      aVariant({ id: 'v-1l', name: '1L' }),
      aVariant({ id: 'v-2l', name: '2L', price_diff: { amount_minor: 200_00, currency: 'PKR' } }),
    ],
  });
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

  describe('a product sold in more than one size', () => {
    it('sells the size the customer picked, not the default', () => {
      // The defect this closes: the variant reached the contract and the
      // client, and the screen never offered one — so a shop selling rice in
      // two sizes could only ever be sold the default line.
      const addItem = jest.fn(async () => {});
      render(<ShopScreen grocery={shopping({ products: [withSizes()], addItem })} />);

      fireEvent.press(screen.getByTestId('product-p-1-variant-v-2l'));
      fireEvent.press(screen.getByTestId('product-p-1-add'));

      expect(addItem).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'p-1' }),
        1,
        'ASK_ME',
        'v-2l',
      );
    });

    it('shows what the chosen size costs, not what the product costs', () => {
      // Document 68 stores a difference rather than a price, so a screen that
      // renders the product's price shows the 2L at the 1L's money.
      render(<ShopScreen grocery={shopping({ products: [withSizes()] })} />);

      expect(screen.getByTestId('product-p-1-price')).toHaveTextContent('PKR 250.00');
      fireEvent.press(screen.getByTestId('product-p-1-variant-v-2l'));
      expect(screen.getByTestId('product-p-1-price')).toHaveTextContent('PKR 450.00');
    });

    it('opens on a size that is actually in stock', () => {
      const addItem = jest.fn(async () => {});
      const product = withSizes();
      product.variants = [
        aVariant({ id: 'v-1l', name: '1L', available: false }),
        aVariant({ id: 'v-2l', name: '2L', available: true }),
      ];
      render(<ShopScreen grocery={shopping({ products: [product], addItem })} />);

      // No tap on a size: the row opened on the one that can be sold.
      fireEvent.press(screen.getByTestId('product-p-1-add'));
      expect(addItem).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'p-1' }),
        1,
        'ASK_ME',
        'v-2l',
      );
    });

    it('shows a size that has run out rather than hiding it', () => {
      // Hiding it tells a customer the shop does not stock the 2L at all,
      // when the truth is that it is out today.
      const addItem = jest.fn(async () => {});
      const product = withSizes();
      product.variants = [
        aVariant({ id: 'v-1l', name: '1L', available: true }),
        aVariant({ id: 'v-2l', name: '2L', available: false }),
      ];
      render(<ShopScreen grocery={shopping({ products: [product], addItem })} />);

      const outOfStock = screen.getByTestId('product-p-1-variant-v-2l');
      expect(outOfStock).toBeTruthy();
      fireEvent.press(outOfStock);
      fireEvent.press(screen.getByTestId('product-p-1-add'));

      // Pressing it changed nothing: the sellable size is still the one added.
      expect(addItem).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'p-1' }),
        1,
        'ASK_ME',
        'v-1l',
      );
    });

    it('is out of stock when every size is', () => {
      // The product row's own flag is about the default line, and a product
      // whose every size has run out cannot be sold whatever it says.
      const product = withSizes();
      product.available = true;
      product.variants = [
        aVariant({ id: 'v-1l', available: false }),
        aVariant({ id: 'v-2l', available: false }),
      ];
      render(<ShopScreen grocery={shopping({ products: [product] })} />);

      expect(screen.queryByTestId('product-p-1-add')).toBeNull();
      expect(screen.getByText('Out of stock')).toBeTruthy();
    });

    it('leaves a shelf with no sizes exactly as it was', () => {
      // Most of a kiryana's shelf. The call it makes must not change because
      // this screen learned about sizes.
      const addItem = jest.fn(async () => {});
      render(<ShopScreen grocery={shopping({ addItem })} />);

      expect(screen.queryByTestId('product-p-1-variants')).toBeNull();
      fireEvent.press(screen.getByTestId('product-p-1-add'));
      expect(addItem).toHaveBeenCalledWith(expect.objectContaining({ id: 'p-1' }), 1, 'ASK_ME');
    });
  });
});
