import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { MerchantOrder } from '@platform/types';
import { ApiError } from '@platform/api-client';
import { useQueue } from './useQueue';

function order(id: string, status = 'PLACED'): MerchantOrder {
  return {
    id,
    status,
    items_total: { amount_minor: 1000, currency: 'PKR' },
    created_at: '2026-09-11T09:00:00Z',
  } as MerchantOrder;
}

afterEach(() => vi.useRealTimers());

describe('useQueue', () => {
  it('asks for the queue it was given', async () => {
    const listMerchantOrders = vi.fn(async () => ({ items: [order('o1')] }));
    const { result } = renderHook(() => useQueue({ listMerchantOrders } as never, 'preparing'));

    await waitFor(() => expect(result.current.orders).toHaveLength(1));
    expect(listMerchantOrders).toHaveBeenCalledWith(
      expect.objectContaining({ queue: 'preparing' }),
    );
  });

  it('empties the list when the queue changes, rather than showing the old one under the new heading', async () => {
    const listMerchantOrders = vi
      .fn()
      .mockResolvedValueOnce({ items: [order('o1')] })
      .mockImplementation(() => new Promise(() => {}));

    const { result, rerender } = renderHook(
      ({ queue }) => useQueue({ listMerchantOrders } as never, queue),
      {
        initialProps: { queue: 'new' as const },
      },
    );
    await waitFor(() => expect(result.current.orders).toHaveLength(1));

    rerender({ queue: 'ready' as never });
    expect(result.current.orders).toEqual([]);
  });

  it('polls without disturbing the list it already has', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const listMerchantOrders = vi.fn(async () => ({ items: [order('o1')] }));
    const { result } = renderHook(() => useQueue({ listMerchantOrders } as never, 'new', 25, 1000));

    await waitFor(() => expect(result.current.orders).toHaveLength(1));
    expect(listMerchantOrders).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(listMerchantOrders).toHaveBeenCalledTimes(2);
    // A poll must not flash the spinner every fifteen seconds.
    expect(result.current.loading).toBe(false);
  });

  it('appends the next page rather than replacing it', async () => {
    const listMerchantOrders = vi
      .fn()
      .mockResolvedValueOnce({ items: [order('o1')], nextCursor: 'c1' })
      .mockResolvedValueOnce({ items: [order('o2')] });
    const { result } = renderHook(() => useQueue({ listMerchantOrders } as never, 'new', 25, 0));

    await waitFor(() => expect(result.current.hasMore).toBe(true));
    await act(async () => {
      await result.current.loadMore();
    });

    expect(result.current.orders.map((o) => o.id)).toEqual(['o1', 'o2']);
    expect(result.current.hasMore).toBe(false);
  });

  it('reports a failure in words a shop can act on', async () => {
    const listMerchantOrders = vi.fn(async () => {
      throw new ApiError(0, { code: 'unavailable', message: 'network', request_id: '' });
    });
    const { result } = renderHook(() => useQueue({ listMerchantOrders } as never, 'new', 25, 0));

    await waitFor(() => expect(result.current.error).toContain('connection'));
  });
});
