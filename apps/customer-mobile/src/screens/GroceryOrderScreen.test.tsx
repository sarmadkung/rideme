import { fireEvent, render, screen } from '@testing-library/react-native';
import type { GroceryOrder, OrderIssue } from '@platform/types';
import { GroceryOrderScreen, statusLabel } from './GroceryOrderScreen';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

function anIssue(overrides: Partial<OrderIssue> = {}): OrderIssue {
  return {
    id: 'issue-1',
    order_item_id: 'line-1',
    reason: 'Out of stock',
    action: 'REQUEST_CUSTOMER_DECISION',
    resolution: 'PENDING',
    substitute_name: 'Guard rice 5kg',
    substitute_price: { amount_minor: 400_00, currency: 'PKR' },
    price_difference: { amount_minor: 150_00, currency: 'PKR' },
    created_at: '2026-09-12T09:00:00Z',
    ...overrides,
  } as OrderIssue;
}

function anOrder(status: string, overrides: Partial<GroceryOrder> = {}): GroceryOrder {
  return {
    id: 'order-1',
    status,
    items_total: { amount_minor: 250_00, currency: 'PKR' },
    items: [
      {
        id: 'line-1',
        name: 'Basmati rice 5kg',
        quantity: 1,
        unit_price: { amount_minor: 250_00, currency: 'PKR' },
        line_total: { amount_minor: 250_00, currency: 'PKR' },
        substitution_preference: 'ASK_ME',
        status: 'ORDERED',
      },
    ],
    created_at: '2026-09-12T09:00:00Z',
    ...overrides,
  } as GroceryOrder;
}

function tracking(
  order: GroceryOrder,
  overrides: Partial<GroceryState & GroceryActions> = {},
): GroceryState & GroceryActions {
  return {
    stage: 'tracking',
    stores: [],
    store: null,
    products: [],
    order,
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

describe('GroceryOrderScreen', () => {
  it('reads the lifecycle in the customer’s words', () => {
    render(<GroceryOrderScreen grocery={tracking(anOrder('PREPARING'))} />);
    expect(screen.getByTestId('order-status')).toHaveTextContent('Being picked');
    // READY_FOR_PICKUP is the shop's word for it. The customer is waiting for
    // a rider, and that is what the screen says.
    expect(statusLabel('READY_FOR_PICKUP')).toBe('Waiting for a rider');
  });

  it('puts a pending substitution above everything else, with both prices', () => {
    // A picker is standing at a shelf waiting for this answer. BD-11 charges
    // the substitute's actual price, so the difference is shown either way.
    render(
      <GroceryOrderScreen grocery={tracking(anOrder('PREPARING', { issues: [anIssue()] }))} />,
    );

    const question = screen.getByTestId('order-question-substitute');
    expect(question).toHaveTextContent(/Guard rice 5kg/);
    expect(question).toHaveTextContent(/PKR 400\.00/);
    expect(question).toHaveTextContent(/PKR 150\.00/);
  });

  it('sends the answer the customer gave', () => {
    const decide = jest.fn(async () => {});
    render(
      <GroceryOrderScreen
        grocery={tracking(anOrder('PREPARING', { issues: [anIssue()] }), { decide })}
      />,
    );

    fireEvent.press(screen.getByTestId('order-question-decline'));
    expect(decide).toHaveBeenCalledWith('issue-1', false);
  });

  it('asks nothing when the shop has already been answered', () => {
    const settled = anIssue({ resolution: 'CUSTOMER_ACCEPTED' });
    render(<GroceryOrderScreen grocery={tracking(anOrder('PREPARING', { issues: [settled] }))} />);
    expect(screen.queryByTestId('order-question')).toBeNull();
  });

  it('offers cancellation before picking starts and not after', () => {
    render(<GroceryOrderScreen grocery={tracking(anOrder('PLACED'))} />);
    expect(screen.getByTestId('order-cancel')).toBeTruthy();

    screen.rerender(<GroceryOrderScreen grocery={tracking(anOrder('PREPARING'))} />);
    // The service refuses it once a picker has walked the aisles. Offering the
    // button anyway produces a refusal the customer can do nothing about.
    expect(screen.queryByTestId('order-cancel')).toBeNull();
  });

  it('strikes through a line the shop could not supply', () => {
    const order = anOrder('PREPARING', {
      items: [
        {
          id: 'line-1',
          name: 'Basmati rice 5kg',
          quantity: 1,
          unit_price: { amount_minor: 250_00, currency: 'PKR' },
          line_total: { amount_minor: 0, currency: 'PKR' },
          substitution_preference: 'DO_NOT_ALLOW',
          status: 'REMOVED',
        },
      ],
    } as Partial<GroceryOrder>);
    render(<GroceryOrderScreen grocery={tracking(order)} />);
    expect(screen.getByTestId('order-line-line-1')).toHaveTextContent(/not available/);
  });
});
