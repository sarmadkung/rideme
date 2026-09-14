import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ApiClient } from '@platform/api-client';
import type { Store } from '@platform/types';
import { StoresScreen, formatDistance } from './StoresScreen';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

function aStore(overrides: Partial<Store> = {}): Store {
  return {
    id: 'store-1',
    merchant_name: 'Al-Fatah',
    name: 'Gulberg',
    latitude: 31.5,
    longitude: 74.3,
    distance_m: 400,
    open: true,
    ...overrides,
  } as Store;
}

function browsing(
  overrides: Partial<GroceryState & GroceryActions> = {},
): GroceryState & GroceryActions {
  return {
    stage: 'stores',
    stores: [aStore()],
    store: null,
    products: [],
    order: null,
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

describe('StoresScreen', () => {
  it('looks for shops without waiting to be told where', () => {
    // The app has no location permission, so "near me" has to be asked rather
    // than sensed. Starting from a known point puts shops on the first screen
    // instead of an empty state.
    const findStores = jest.fn(async () => {});
    render(<StoresScreen grocery={browsing({ findStores })} client={client} />);
    expect(findStores).toHaveBeenCalledWith(
      expect.objectContaining({ latitude: expect.any(Number) }),
    );
  });

  it('lists a closed shop and refuses to open it', () => {
    // Hiding it would tell a customer their usual kiryana had vanished;
    // letting them fill a cart in it wastes their time and ends in rejection.
    const enterStore = jest.fn(async () => {});
    render(
      <StoresScreen
        grocery={browsing({ stores: [aStore({ open: false })], enterStore })}
        client={client}
      />,
    );

    expect(screen.getByTestId('store-store-1')).toBeTruthy();
    fireEvent.press(screen.getByTestId('store-store-1'));
    expect(enterStore).not.toHaveBeenCalled();
  });

  it('opens an open shop', () => {
    const enterStore = jest.fn(async () => {});
    render(<StoresScreen grocery={browsing({ enterStore })} client={client} />);
    fireEvent.press(screen.getByTestId('store-store-1'));
    expect(enterStore).toHaveBeenCalledWith(expect.objectContaining({ id: 'store-1' }));
  });

  it('says nothing delivers here rather than showing an empty page', () => {
    render(<StoresScreen grocery={browsing({ stores: [] })} client={client} />);
    expect(screen.getByTestId('stores-empty')).toBeTruthy();
  });
});

describe('formatDistance', () => {
  it('uses metres below a kilometre and one decimal above it', () => {
    // Nobody walks 1,240 m.
    expect(formatDistance(400)).toBe('400 m');
    expect(formatDistance(1240)).toBe('1.2 km');
  });
});
