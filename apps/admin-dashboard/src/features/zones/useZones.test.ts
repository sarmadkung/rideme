import { describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import type { Zone } from '@platform/types';
import { ApiError } from '@platform/api-client';
import { useZones } from './useZones';

function zone(id: string, name: string): Zone {
  return {
    id,
    name,
    latitude: 31.52,
    longitude: 74.35,
    radius_meters: 2000,
    status: 'ACTIVE',
    created_at: '2026-08-28T12:00:00Z',
  };
}

describe('useZones', () => {
  it('loads zones on mount', async () => {
    const client = { listZones: vi.fn(async () => [zone('z1', 'Liberty Market')]) } as never;

    const { result } = renderHook(() => useZones(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.zones).toHaveLength(1);
    expect(result.current.zones[0]?.name).toBe('Liberty Market');
  });

  it('surfaces a load failure without crashing', async () => {
    const client = {
      listZones: vi.fn(async () => {
        throw new ApiError(500, { code: 'internal', message: 'boom', request_id: 'r' });
      }),
    } as never;

    const { result } = renderHook(() => useZones(client));
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.zones).toHaveLength(0);
  });

  it('creates a zone and refreshes the list', async () => {
    const listZones = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([zone('z1', 'Liberty Market')]);
    const createZone = vi.fn(async () => zone('z1', 'Liberty Market'));
    const client = { listZones, createZone } as never;

    const { result } = renderHook(() => useZones(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    let ok = false;
    await act(async () => {
      ok = await result.current.create({
        name: 'Liberty Market',
        latitude: 31.52,
        longitude: 74.35,
        radiusMeters: 2000,
      });
    });

    expect(ok).toBe(true);
    expect(createZone).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Liberty Market', radiusMeters: 2000 }),
    );
    await waitFor(() => expect(result.current.zones).toHaveLength(1));
  });

  it('reports a 403 as a permission error rather than a generic failure', async () => {
    const client = {
      listZones: vi.fn(async () => []),
      createZone: vi.fn(async () => {
        throw new ApiError(403, { code: 'forbidden', message: 'not permitted', request_id: 'r' });
      }),
    } as never;

    const { result } = renderHook(() => useZones(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.create({ name: 'x', latitude: 0, longitude: 0, radiusMeters: 100 });
    });

    expect(result.current.createError).toMatch(/permission/i);
  });
});
