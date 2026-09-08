import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiClient, Place } from '@platform/api-client';

/**
 * Free-text place search for the "where to?" field.
 *
 * Every search is a billed call on the server, so this does not send one per
 * keystroke. It waits for the customer to stop typing, and it discards an
 * answer that arrives after a newer one — a slow response to "Lib" must not
 * overwrite the results for "Liberty Market".
 */

export type SearchStage =
  /** Nothing typed, or too little to be worth a call. */
  | 'idle'
  | 'searching'
  /** The provider answered, possibly with nothing. */
  | 'done'
  /** The provider could not be asked. Not the same as finding nothing. */
  | 'failed';

export interface PlaceSearchState {
  query: string;
  results: Place[];
  stage: SearchStage;
  /** Set only when the search itself failed. */
  error: string | null;
}

export interface PlaceSearchActions {
  setQuery(query: string): void;
  clear(): void;
}

/**
 * How long the customer must stop typing before a call is sent.
 *
 * Document 104 asks to "debounce search" among its cost controls. 300ms is
 * about a fast typist's gap between words: long enough that "Liberty" is one
 * call rather than seven, short enough that the list does not feel stuck.
 */
export const SEARCH_DEBOUNCE_MS = 300;

/**
 * Below this, a search is not worth a billed call.
 *
 * One or two characters match most of the city and tell the customer nothing
 * they did not already know.
 */
export const MIN_QUERY_LENGTH = 3;

export function usePlaceSearch(
  client: ApiClient,
  near?: { latitude: number; longitude: number },
): PlaceSearchState & PlaceSearchActions {
  const [query, setQueryState] = useState('');
  const [results, setResults] = useState<Place[]>([]);
  const [stage, setStage] = useState<SearchStage>('idle');
  const [error, setError] = useState<string | null>(null);

  // Identifies the most recent search so a slower earlier one can be dropped.
  const latest = useRef(0);

  // Held in a ref, and deliberately not an effect dependency. A caller that
  // rebuilds its client on each render would otherwise restart the search on
  // every keystroke's re-render, which is an infinite loop rather than a
  // debounce.
  const clientRef = useRef(client);
  clientRef.current = client;

  const setQuery = useCallback((next: string) => setQueryState(next), []);

  const clear = useCallback(() => {
    latest.current += 1;
    setQueryState('');
    setResults([]);
    setStage('idle');
    setError(null);
  }, []);

  const latitude = near?.latitude;
  const longitude = near?.longitude;

  useEffect(() => {
    const trimmed = query.trim();
    if (trimmed.length < MIN_QUERY_LENGTH) {
      // Not an error, and not worth a call. Any in-flight answer is now stale.
      latest.current += 1;
      setResults([]);
      setStage('idle');
      setError(null);
      return;
    }

    const search = ++latest.current;
    setStage('searching');

    const timer = setTimeout(() => {
      void (async () => {
        try {
          const found = await clientRef.current.searchPlaces(
            trimmed,
            latitude !== undefined && longitude !== undefined ? { latitude, longitude } : undefined,
          );
          if (search !== latest.current) return;
          setResults(found);
          setStage('done');
          setError(null);
        } catch {
          if (search !== latest.current) return;
          setResults([]);
          setStage('failed');
          // Deliberately not the server's message: it can name a disabled API
          // or a restricted key, which is an operator's problem.
          setError('Search is unavailable. Pick a place below, or try again.');
        }
      })();
    }, SEARCH_DEBOUNCE_MS);

    return () => clearTimeout(timer);
  }, [query, latitude, longitude]);

  return { query, results, stage, error, setQuery, clear };
}
