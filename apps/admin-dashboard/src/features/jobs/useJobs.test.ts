import { describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import type { Job } from '@platform/types';
import { ApiError } from '@platform/api-client';
import { triage, useJobs } from './useJobs';

function job(id: string, status: string): Job {
  return {
    id,
    type: 'RIDE',
    status,
    stops: [{ id: `${id}-s0`, sequence: 0, type: 'PICKUP', latitude: 31.52, longitude: 74.35 }],
    created_at: '2026-08-28T12:00:00Z',
  } as Job;
}

describe('useJobs', () => {
  it('loads the first page on mount', async () => {
    const client = {
      listJobs: vi.fn(async () => ({ items: [job('j1', 'SEARCHING')] })),
    } as never;

    const { result } = renderHook(() => useJobs(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.jobs).toHaveLength(1);
    expect(result.current.hasMore).toBe(false);
  });

  it('appends the next page rather than replacing it', async () => {
    const listJobs = vi
      .fn()
      .mockResolvedValueOnce({ items: [job('j1', 'SEARCHING')], nextCursor: 'c1' })
      .mockResolvedValueOnce({ items: [job('j2', 'COMPLETED')] });
    const client = { listJobs } as never;

    const { result } = renderHook(() => useJobs(client));
    await waitFor(() => expect(result.current.jobs).toHaveLength(1));
    expect(result.current.hasMore).toBe(true);

    await act(async () => {
      await result.current.loadMore();
    });

    expect(result.current.jobs.map((j) => j.id)).toEqual(['j1', 'j2']);
    expect(result.current.hasMore).toBe(false);
  });

  it('does not load more when there is no cursor', async () => {
    // Without the guard an operator scrolling at the end of the list would
    // re-request the last page indefinitely.
    const listJobs = vi.fn(async () => ({ items: [job('j1', 'COMPLETED')] }));
    const client = { listJobs } as never;

    const { result } = renderHook(() => useJobs(client));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.loadMore();
    });
    expect(listJobs).toHaveBeenCalledTimes(1);
  });

  it('tells an operator when their session ended rather than showing a raw error', async () => {
    const client = {
      listJobs: vi.fn(async () => {
        throw new ApiError(401, { code: 'unauthorized', message: 'expired', request_id: 'r' });
      }),
    } as never;

    const { result } = renderHook(() => useJobs(client));
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error).toMatch(/sign in again/i);
  });
});

describe('triage', () => {
  it('separates what needs attention from what is running and what is done', () => {
    const { searching, active, finished } = triage([
      job('a', 'SEARCHING'),
      job('b', 'IN_PROGRESS'),
      job('c', 'COMPLETED'),
      job('d', 'REQUESTED'),
      job('e', 'CANCELLED'),
      job('f', 'ACCEPTED'),
    ]);

    // A job nobody has taken is the one an operator must act on.
    expect(searching.map((j) => j.id)).toEqual(['a', 'd']);
    expect(active.map((j) => j.id)).toEqual(['b', 'f']);
    expect(finished.map((j) => j.id)).toEqual(['c', 'e']);
  });

  it('treats every terminal state as finished', () => {
    const { finished } = triage([
      job('a', 'COMPLETED'),
      job('b', 'CANCELLED'),
      job('c', 'FAILED'),
      job('d', 'EXPIRED'),
      job('e', 'DISPUTED'),
    ]);
    expect(finished).toHaveLength(5);
  });
});
