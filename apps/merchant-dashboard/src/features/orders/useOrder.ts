import { useCallback, useEffect, useState } from 'react';
import type { ApiClient, ItemIssueInput } from '@platform/api-client';
import type { MerchantOrder } from '@platform/types';
import { describeFailure } from './actions';

export interface OrderState {
  order: MerchantOrder | null;
  loading: boolean;
  /** An action is in flight. The buttons are disabled, not hidden. */
  working: boolean;
  error: string | null;
}

export interface OrderActions {
  accept(): Promise<void>;
  reject(reason: string): Promise<void>;
  startPreparing(): Promise<void>;
  markReady(): Promise<void>;
  reportIssue(itemId: string, issue: ItemIssueInput): Promise<void>;
  reload(): Promise<void>;
}

/**
 * One order, and the five things a merchant can do to it (document 72).
 *
 * Every action returns the order as the server now holds it, and that answer
 * replaces local state wholesale. The screen never predicts the next status:
 * Accept on an order the customer has just cancelled must show cancelled, not
 * accepted, and the only way to be sure is to render what came back.
 */
export function useOrder(
  client: ApiClient,
  orderId: string | null,
  onChanged?: () => void,
): OrderState & OrderActions {
  const [order, setOrder] = useState<MerchantOrder | null>(null);
  const [loading, setLoading] = useState(false);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (!orderId) {
      setOrder(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setOrder(await client.getMerchantOrder(orderId));
    } catch (cause) {
      setError(describeFailure(cause));
    } finally {
      setLoading(false);
    }
  }, [client, orderId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const run = useCallback(
    async (action: () => Promise<MerchantOrder>) => {
      setWorking(true);
      setError(null);
      try {
        setOrder(await action());
        // The queue this order sits in has usually just changed too.
        onChanged?.();
      } catch (cause) {
        const failure = describeFailure(cause);
        // A conflict means somebody else moved it. Showing the shop a stale
        // order beside the reason it could not be moved invites a second tap
        // on a button that cannot work.
        //
        // The order of these two lines is the whole point: `reload` clears the
        // error before it fetches, so setting the message first would leave
        // the shop with a silently corrected order and no idea why their tap
        // did nothing.
        await reload();
        setError(failure);
      } finally {
        setWorking(false);
      }
    },
    [onChanged, reload],
  );

  return {
    order,
    loading,
    working,
    error,
    reload,
    accept: useCallback(
      () => run(() => client.acceptMerchantOrder(orderId ?? '')),
      [client, orderId, run],
    ),
    reject: useCallback(
      (reason: string) => run(() => client.rejectMerchantOrder(orderId ?? '', reason)),
      [client, orderId, run],
    ),
    startPreparing: useCallback(
      () => run(() => client.startPreparingMerchantOrder(orderId ?? '')),
      [client, orderId, run],
    ),
    markReady: useCallback(
      () => run(() => client.markMerchantOrderReady(orderId ?? '')),
      [client, orderId, run],
    ),
    reportIssue: useCallback(
      async (itemId: string, issue: ItemIssueInput) => {
        // The issue response is one issue, not the order — and reporting one
        // can reprice the order (BD-11), so the order is read again rather
        // than patched locally.
        await run(async () => {
          await client.reportMerchantItemIssue(orderId ?? '', itemId, issue);
          return client.getMerchantOrder(orderId ?? '');
        });
      },
      [client, orderId, run],
    ),
  };
}
