import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiClient } from '@platform/api-client';
import type { MerchantOrder, MerchantQueue } from '@platform/types';
import { describeFailure } from './actions';

/**
 * How often a queue re-reads itself.
 *
 * Document 77 asks for WebSockets here, and the realtime hub this would use
 * has no transport yet (Phase 6 built the hub; nothing binds it to a socket).
 * Polling is the stand-in, not the design: a New queue that only changes when
 * somebody clicks Refresh is a queue that loses orders, and BD-12 cancels an
 * unanswered one in ten minutes. Fifteen seconds costs four requests a minute
 * per open terminal, which is affordable for one shop and is the first thing
 * the socket should replace.
 */
export const POLL_MS = 15_000;

export interface QueueState {
  orders: MerchantOrder[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
}

export interface QueueActions {
  refresh(): Promise<void>;
  loadMore(): Promise<void>;
}

export function useQueue(
  client: ApiClient,
  queue: MerchantQueue,
  pageSize = 25,
  pollMs = POLL_MS,
): QueueState & QueueActions {
  const [orders, setOrders] = useState<MerchantOrder[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  // Polling must not fight the operator: a background tick while they are
  // pulling the next page would replace the list under them.
  const busy = useRef(false);

  const load = useCallback(
    async (from: string | undefined, append: boolean, quiet: boolean) => {
      if (busy.current) return;
      busy.current = true;
      if (!quiet) setLoading(true);
      try {
        const page = await client.listMerchantOrders({
          queue,
          limit: pageSize,
          ...(from ? { cursor: from } : {}),
        });
        setOrders((current) => (append ? [...current, ...page.items] : page.items));
        setCursor(page.nextCursor);
        setHasMore(Boolean(page.nextCursor));
        setError(null);
      } catch (cause) {
        setError(describeFailure(cause));
      } finally {
        busy.current = false;
        if (!quiet) setLoading(false);
      }
    },
    [client, queue, pageSize],
  );

  const refresh = useCallback(() => load(undefined, false, false), [load]);
  const loadMore = useCallback(async () => {
    if (!cursor) return;
    await load(cursor, true, false);
  }, [cursor, load]);

  useEffect(() => {
    // A queue change is a different list, so the old one goes immediately
    // rather than lingering under the new tab's heading.
    setOrders([]);
    setCursor(undefined);
    setHasMore(false);
    void load(undefined, false, false);
  }, [load]);

  useEffect(() => {
    if (pollMs <= 0) return;
    const timer = setInterval(() => {
      // Quiet: a poll must not flash the spinner every fifteen seconds.
      void load(undefined, false, true);
    }, pollMs);
    return () => clearInterval(timer);
  }, [load, pollMs]);

  return { orders, loading, error, hasMore, refresh, loadMore };
}
