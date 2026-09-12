import { describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { MerchantOrder } from '@platform/types';
import { ApiError } from '@platform/api-client';
import { useOrder } from './useOrder';

function order(status: string): MerchantOrder {
  return {
    id: 'o1',
    status,
    items_total: { amount_minor: 1000, currency: 'PKR' },
    created_at: '2026-09-11T09:00:00Z',
  } as MerchantOrder;
}

describe('useOrder', () => {
  it('renders the order the server returned, not the one the screen expected', async () => {
    // Accept on an order the customer cancelled a second ago must show
    // cancelled. The only way to be sure is to render the answer.
    const client = {
      getMerchantOrder: vi.fn(async () => order('PLACED')),
      acceptMerchantOrder: vi.fn(async () => order('CANCELLED')),
    } as never;

    const { result } = renderHook(() => useOrder(client, 'o1'));
    await waitFor(() => expect(result.current.order?.status).toBe('PLACED'));

    await act(async () => {
      await result.current.accept();
    });

    expect(result.current.order?.status).toBe('CANCELLED');
  });

  it('reloads after a refused action so the buttons match the order', async () => {
    // The refusal means somebody else moved it. Leaving the stale order on
    // screen beside the reason invites a second tap on a button that cannot
    // work.
    const getMerchantOrder = vi
      .fn()
      .mockResolvedValueOnce(order('PLACED'))
      .mockResolvedValueOnce(order('CONFIRMED'));
    const client = {
      getMerchantOrder,
      acceptMerchantOrder: vi.fn(async () => {
        throw new ApiError(409, {
          code: 'conflict',
          message: 'this order is not waiting to be accepted',
          request_id: 'r1',
        });
      }),
    } as never;

    const { result } = renderHook(() => useOrder(client, 'o1'));
    await waitFor(() => expect(result.current.order?.status).toBe('PLACED'));

    await act(async () => {
      await result.current.accept();
    });

    expect(result.current.error).toBe('this order is not waiting to be accepted');
    expect(result.current.order?.status).toBe('CONFIRMED');
    expect(getMerchantOrder).toHaveBeenCalledTimes(2);
  });

  it('tells the queue beside it that the order moved', async () => {
    const onChanged = vi.fn();
    const client = {
      getMerchantOrder: vi.fn(async () => order('CONFIRMED')),
      startPreparingMerchantOrder: vi.fn(async () => order('PREPARING')),
    } as never;

    const { result } = renderHook(() => useOrder(client, 'o1', onChanged));
    await waitFor(() => expect(result.current.order?.status).toBe('CONFIRMED'));

    await act(async () => {
      await result.current.startPreparing();
    });

    expect(onChanged).toHaveBeenCalled();
  });

  it('re-reads the order after an issue is reported, because reporting one reprices it', async () => {
    // BD-11: an applied substitution changes the total. The issue response is
    // one issue, not the order, so patching locally would show the old money.
    const getMerchantOrder = vi
      .fn()
      .mockResolvedValueOnce(order('PREPARING'))
      .mockResolvedValueOnce({
        ...order('PREPARING'),
        items_total: { amount_minor: 1400, currency: 'PKR' },
      });
    const reportMerchantItemIssue = vi.fn(async () => ({ id: 'i1' }));
    const client = { getMerchantOrder, reportMerchantItemIssue } as never;

    const { result } = renderHook(() => useOrder(client, 'o1'));
    await waitFor(() => expect(result.current.order).not.toBeNull());

    await act(async () => {
      await result.current.reportIssue('item-1', { reason: 'empty shelf', action: 'REMOVE' });
    });

    expect(reportMerchantItemIssue).toHaveBeenCalledWith('o1', 'item-1', {
      reason: 'empty shelf',
      action: 'REMOVE',
    });
    expect(result.current.order?.items_total.amount_minor).toBe(1400);
  });
});
