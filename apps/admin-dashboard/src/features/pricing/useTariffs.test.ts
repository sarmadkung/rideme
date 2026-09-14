import { describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import type { Tariff } from '@platform/types';
import { ApiError } from '@platform/api-client';
import { useTariffs } from './useTariffs';

function tariff(id: string, baseMinor: number): Tariff {
  return {
    id,
    job_type: 'RIDE',
    version: 1,
    currency: 'PKR',
    minimum_fare_minor: 15000,
    base_minor: baseMinor,
    per_km_minor: 3000,
    per_minute_minor: 300,
    waiting_per_minute_minor: 0,
    loading_per_minute_minor: 0,
    per_kg_minor: 0,
    service_fee_minor: 0,
    service_fee_bps: 0,
    tax_bps: 0,
  };
}

describe('useTariffs', () => {
  it('loads tariffs on mount', async () => {
    const client = { listTariffs: vi.fn(async () => [tariff('t1', 10000)]) } as never;

    const { result } = renderHook(() => useTariffs(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.tariffs).toHaveLength(1);
    expect(result.current.tariffs[0]?.base_minor).toBe(10000);
  });

  it('creates a tariff and refreshes the list', async () => {
    const listTariffs = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([tariff('t1', 10000)]);
    const createTariff = vi.fn(async () => tariff('t1', 10000));
    const client = { listTariffs, createTariff } as never;

    const { result } = renderHook(() => useTariffs(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    let ok = false;
    await act(async () => {
      ok = await result.current.create({
        jobType: 'RIDE',
        version: 1,
        minimumFareMinor: 15000,
        baseMinor: 10000,
        perKmMinor: 3000,
        perMinuteMinor: 300,
      });
    });

    expect(ok).toBe(true);
    expect(createTariff).toHaveBeenCalledWith(expect.objectContaining({ jobType: 'RIDE' }));
    await waitFor(() => expect(result.current.tariffs).toHaveLength(1));
  });

  it('reports a 403 as a permission error rather than a generic failure', async () => {
    const client = {
      listTariffs: vi.fn(async () => []),
      createTariff: vi.fn(async () => {
        throw new ApiError(403, { code: 'forbidden', message: 'not permitted', request_id: 'r' });
      }),
    } as never;

    const { result } = renderHook(() => useTariffs(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.create({
        jobType: 'RIDE',
        version: 1,
        minimumFareMinor: 0,
        baseMinor: 0,
        perKmMinor: 0,
        perMinuteMinor: 0,
      });
    });

    expect(result.current.createError).toMatch(/permission/i);
  });
});
