import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError, type ApiClient } from '@platform/api-client';
import type { Place } from '@platform/types';
import { MIN_QUERY_LENGTH, SEARCH_DEBOUNCE_MS, usePlaceSearch } from './usePlaceSearch';

function aPlace(name: string): Place {
  return { name, address: `${name}, Lahore`, point: { lat: 31.5169, lon: 74.3484 } };
}

function client(searchPlaces: ApiClient['searchPlaces']): ApiClient {
  return { searchPlaces } as unknown as ApiClient;
}

const HERE = { latitude: 31.5204, longitude: 74.3587 };

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

/** Advances past the debounce and lets the resulting promise settle. */
async function settle() {
  await act(async () => {
    jest.advanceTimersByTime(SEARCH_DEBOUNCE_MS + 1);
  });
}

it('survives a caller that rebuilds its client every render', async () => {
  // A fresh client identity per render must not restart the search: that is an
  // infinite loop, not a debounce. This crashed the test runner before the
  // client was held in a ref.
  const search = jest.fn(async () => [aPlace('Liberty Market')]);
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Liberty'));
  await settle();
  expect(search).toHaveBeenCalledTimes(1);
});

it('does not call the server on every keystroke', async () => {
  // Every search is a billed call; document 104 asks to debounce.
  const search = jest.fn(async () => [aPlace('Liberty Market')]);
  const stable = client(search);
  const { result } = renderHook(() => usePlaceSearch(stable, HERE));

  for (const partial of ['Lib', 'Libe', 'Liber', 'Libert', 'Liberty']) {
    act(() => result.current.setQuery(partial));
  }
  await settle();

  expect(search).toHaveBeenCalledTimes(1);
  expect(search).toHaveBeenCalledWith('Liberty', HERE);
});

it('does not search for one or two characters', async () => {
  // They match most of the city and tell the customer nothing.
  const search = jest.fn(async () => []);
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Li'));
  await settle();

  expect(search).not.toHaveBeenCalled();
  expect(result.current.stage).toBe('idle');
  expect(MIN_QUERY_LENGTH).toBeGreaterThan(2);
});

it('ignores a slow answer to an older query', async () => {
  // A late response to "Lib" must not overwrite the results for "Liberty".
  const resolvers: Array<(places: Place[]) => void> = [];
  const search = jest.fn(() => new Promise<Place[]>((resolve) => resolvers.push(resolve)));
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Lib'));
  await settle();
  act(() => result.current.setQuery('Liberty'));
  await settle();
  expect(search).toHaveBeenCalledTimes(2);

  // The newer answer lands first, then the stale one.
  await act(async () => {
    resolvers[1]?.([aPlace('Liberty Market')]);
  });
  await act(async () => {
    resolvers[0]?.([aPlace('Library')]);
  });

  expect(result.current.results.map((p) => p.name)).toEqual(['Liberty Market']);
});

it('reports finding nothing as an answer, not a failure', async () => {
  const search = jest.fn(async () => []);
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('asdfghjkl'));
  await settle();

  await waitFor(() => expect(result.current.stage).toBe('done'));
  expect(result.current.results).toEqual([]);
  expect(result.current.error).toBeNull();
});

it('distinguishes an outage from an empty result', async () => {
  // A customer shown "no results" during an outage retypes their address
  // until they give up.
  const search = jest.fn(async () => {
    throw new ApiError(503, {
      code: 'unavailable',
      message: 'search is unavailable right now',
      request_id: 'r1',
    });
  });
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Liberty'));
  await settle();

  await waitFor(() => expect(result.current.stage).toBe('failed'));
  expect(result.current.error).toMatch(/unavailable/i);
});

it('searches without a position when the customer has none yet', async () => {
  const search = jest.fn(async () => [aPlace('Lahore Airport')]);
  const { result } = renderHook(() => usePlaceSearch(client(search), undefined));

  act(() => result.current.setQuery('Lahore Airport'));
  await settle();

  expect(search).toHaveBeenCalledWith('Lahore Airport', undefined);
});

it('clearing stops an in-flight search from landing', async () => {
  const resolvers: Array<(places: Place[]) => void> = [];
  const search = jest.fn(() => new Promise<Place[]>((resolve) => resolvers.push(resolve)));
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Liberty'));
  await settle();
  act(() => result.current.clear());

  await act(async () => {
    resolvers[0]?.([aPlace('Liberty Market')]);
  });

  expect(result.current.results).toEqual([]);
  expect(result.current.query).toBe('');
  expect(result.current.stage).toBe('idle');
});

it('emptying the box clears the list rather than leaving stale results', async () => {
  const search = jest.fn(async () => [aPlace('Liberty Market')]);
  const { result } = renderHook(() => usePlaceSearch(client(search), HERE));

  act(() => result.current.setQuery('Liberty'));
  await settle();
  await waitFor(() => expect(result.current.results).toHaveLength(1));

  act(() => result.current.setQuery(''));
  await settle();

  expect(result.current.results).toEqual([]);
  expect(result.current.stage).toBe('idle');
});
