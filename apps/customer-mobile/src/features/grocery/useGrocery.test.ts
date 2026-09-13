import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError, type ApiClient } from '@platform/api-client';
import type { GroceryOrder, Product, Store } from '@platform/types';
import { isCancellable, isFinished, pendingIssue, useGrocery } from './useGrocery';

const HERE = { latitude: 31.5204, longitude: 74.3587 };

function aStore(overrides: Partial<Store> = {}): Store {
  return {
    id: 'store-1',
    merchant_name: 'Al-Fatah',
    name: 'Gulberg',
    latitude: 31.52,
    longitude: 74.35,
    distance_m: 800,
    open: true,
    ...overrides,
  } as Store;
}

function aProduct(overrides: Partial<Product> = {}): Product {
  return {
    id: 'p-1',
    name: 'Basmati rice 5kg',
    price: { amount_minor: 250_00, currency: 'PKR' },
    available: true,
    ...overrides,
  } as Product;
}

function anOrder(status = 'CART', overrides: Partial<GroceryOrder> = {}): GroceryOrder {
  return {
    id: 'order-1',
    store_id: 'store-1',
    status,
    items_total: { amount_minor: 0, currency: 'PKR' },
    items: [],
    created_at: '2026-09-12T09:00:00Z',
    ...overrides,
  } as GroceryOrder;
}

function stubClient(overrides: Partial<Record<string, unknown>> = {}): ApiClient {
  return {
    listStores: jest.fn(async () => [aStore()]),
    storeCatalog: jest.fn(async () => [aProduct()]),
    openCart: jest.fn(async () => anOrder()),
    addCartItem: jest.fn(async () =>
      anOrder('CART', {
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
      }),
    ),
    placeGroceryOrder: jest.fn(async () => anOrder('PLACED')),
    getGroceryOrder: jest.fn(async () => anOrder('PREPARING')),
    decideGroceryIssue: jest.fn(async () => anOrder('PREPARING')),
    cancelGroceryOrder: jest.fn(async () => anOrder('CANCELLED')),
    ...overrides,
  } as never;
}

describe('useGrocery', () => {
  it('walks shop → cart → checkout → placed', async () => {
    const client = stubClient();
    const { result } = renderHook(() => useGrocery(client, { pollMs: 0 }));

    await act(async () => {
      await result.current.findStores(HERE);
    });
    expect(result.current.stores).toHaveLength(1);

    await act(async () => {
      await result.current.enterStore(aStore());
    });
    expect(result.current.stage).toBe('shopping');
    expect(result.current.products).toHaveLength(1);
    // The cart is opened with the catalogue, so the first "add" has somewhere
    // to go.
    expect(result.current.order?.id).toBe('order-1');

    await act(async () => {
      await result.current.addItem(aProduct(), 1);
    });
    // The total is the server's, not a number the app added up.
    expect(result.current.order?.items_total.amount_minor).toBe(25000);

    act(() => result.current.toCheckout());
    expect(result.current.stage).toBe('checkout');

    await act(async () => {
      await result.current.place({ address: '12 Main Blvd', latitude: 31.5, longitude: 74.3 });
    });
    expect(result.current.stage).toBe('tracking');
    expect(result.current.order?.status).toBe('PLACED');
  });

  it('sends the substitution preference the customer chose', async () => {
    // Document 74: the preference is the customer's standing instruction for
    // that line, and the shop's proposal is judged against it. A line added
    // without one is a line the shop cannot act on alone.
    const client = stubClient();
    const { result } = renderHook(() => useGrocery(client, { pollMs: 0 }));
    await act(async () => {
      await result.current.enterStore(aStore());
    });
    await act(async () => {
      await result.current.addItem(aProduct(), 2, 'DO_NOT_ALLOW');
    });

    expect(client.addCartItem).toHaveBeenCalledWith('order-1', {
      productId: 'p-1',
      quantity: 2,
      substitutionPreference: 'DO_NOT_ALLOW',
    });
  });

  it("passes the server's own words through on a conflict", async () => {
    // "there is nothing in this cart yet" is written for this reader and says
    // what to do next; "something went wrong" does not.
    const client = stubClient({
      placeGroceryOrder: jest.fn(async () => {
        throw new ApiError(409, {
          code: 'conflict',
          message: 'there is nothing in this cart yet',
          request_id: 'r1',
        });
      }),
    });
    const { result } = renderHook(() => useGrocery(client, { pollMs: 0 }));
    await act(async () => {
      await result.current.enterStore(aStore());
    });
    act(() => result.current.toCheckout());
    await act(async () => {
      await result.current.place({ address: 'x', latitude: 31.5, longitude: 74.3 });
    });

    expect(result.current.error).toBe('there is nothing in this cart yet');
    // A refused checkout leaves them where they were, holding the reason. The
    // alternative — advancing anyway, or bouncing them back to the shelves —
    // either lies about the order or throws away the message.
    expect(result.current.stage).toBe('checkout');
    expect(result.current.order?.status).toBe('CART');
  });

  it('follows a placed order until it is over', async () => {
    jest.useFakeTimers();
    const getGroceryOrder = jest.fn(async () => anOrder('PREPARING'));
    const client = stubClient({ getGroceryOrder });
    const { result } = renderHook(() => useGrocery(client, { pollMs: 1000 }));

    await act(async () => {
      await result.current.enterStore(aStore());
    });
    await act(async () => {
      await result.current.place({ address: 'x', latitude: 31.5, longitude: 74.3 });
    });

    await act(async () => {
      jest.advanceTimersByTime(1000);
    });
    await waitFor(() => expect(getGroceryOrder).toHaveBeenCalled());
    jest.useRealTimers();
  });

  it('stops polling once the order is finished', async () => {
    jest.useFakeTimers();
    const getGroceryOrder = jest.fn(async () => anOrder('DELIVERED'));
    const client = stubClient({
      getGroceryOrder,
      placeGroceryOrder: jest.fn(async () => anOrder('DELIVERED')),
    });
    const { result } = renderHook(() => useGrocery(client, { pollMs: 1000 }));

    await act(async () => {
      await result.current.enterStore(aStore());
    });
    await act(async () => {
      await result.current.place({ address: 'x', latitude: 31.5, longitude: 74.3 });
    });
    await act(async () => {
      jest.advanceTimersByTime(5000);
    });

    // A delivered order is not going to change again, and a phone polling a
    // finished order is spending a customer's battery on nothing.
    expect(getGroceryOrder).not.toHaveBeenCalled();
    jest.useRealTimers();
  });

  it('answers a substitution and takes the repriced order back', async () => {
    // BD-11: accepting charges the substitute's actual price, up or down, so
    // the answer changes the total and the whole order comes back.
    const decideGroceryIssue = jest.fn(async () =>
      anOrder('PREPARING', { items_total: { amount_minor: 400_00, currency: 'PKR' } }),
    );
    const client = stubClient({ decideGroceryIssue });
    const { result } = renderHook(() => useGrocery(client, { pollMs: 0 }));

    await act(async () => {
      await result.current.enterStore(aStore());
    });
    await act(async () => {
      await result.current.decide('issue-1', true);
    });

    expect(decideGroceryIssue).toHaveBeenCalledWith('order-1', 'issue-1', true);
    expect(result.current.order?.items_total.amount_minor).toBe(40000);
  });
});

describe('isFinished / isCancellable / pendingIssue', () => {
  it('knows which orders are over', () => {
    for (const status of ['DELIVERED', 'CANCELLED', 'FAILED']) {
      expect(isFinished(anOrder(status))).toBe(true);
    }
    expect(isFinished(anOrder('DELIVERING'))).toBe(false);
    expect(isFinished(null)).toBe(false);
  });

  it('offers cancellation only while the service would allow it', () => {
    // Mirrors `CustomerCancellable`. Offering the button after picking starts
    // produces a refusal the customer can do nothing about.
    for (const status of ['CART', 'PLACED', 'PAYMENT_PENDING', 'CONFIRMED']) {
      expect(isCancellable(anOrder(status))).toBe(true);
    }
    for (const status of ['PREPARING', 'READY_FOR_PICKUP', 'PICKED_UP', 'DELIVERING']) {
      expect(isCancellable(anOrder(status))).toBe(false);
    }
  });

  it('finds the question waiting on the customer, and only a pending one', () => {
    const answered = {
      id: 'i-1',
      order_item_id: 'line-1',
      reason: 'out of stock',
      action: 'SUBSTITUTE',
      resolution: 'CUSTOMER_ACCEPTED',
      created_at: '2026-09-12T09:00:00Z',
    };
    const asking = { ...answered, id: 'i-2', resolution: 'PENDING' };

    expect(pendingIssue(anOrder('PREPARING', { issues: [answered] }))).toBeNull();
    expect(pendingIssue(anOrder('PREPARING', { issues: [answered, asking] }))?.id).toBe('i-2');
    expect(pendingIssue(anOrder('PREPARING'))).toBeNull();
  });
});
