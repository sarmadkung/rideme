import { useCallback, useEffect, useState } from 'react';
import type { ApiClient } from '@platform/api-client';
import { ApiError } from '@platform/api-client';
import type { Job } from '@platform/types';

/**
 * The operational job list, with cursor pagination.
 *
 * The dashboard reads the same endpoints the mobile apps do, through the same
 * generated client. An operator console with its own API layer drifts from the
 * apps it is meant to explain.
 */
export interface JobsState {
  jobs: Job[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
}

export interface JobsActions {
  refresh(): Promise<void>;
  loadMore(): Promise<void>;
}

export function useJobs(client: ApiClient, pageSize = 25): JobsState & JobsActions {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);

  const load = useCallback(
    async (from: string | undefined, append: boolean) => {
      setLoading(true);
      setError(null);
      try {
        const page = await client.listJobs({ limit: pageSize, ...(from ? { cursor: from } : {}) });
        setJobs((current) => (append ? [...current, ...page.items] : page.items));
        setCursor(page.nextCursor);
        setHasMore(Boolean(page.nextCursor));
      } catch (cause) {
        setError(describe(cause));
      } finally {
        setLoading(false);
      }
    },
    [client, pageSize],
  );

  const refresh = useCallback(() => load(undefined, false), [load]);
  const loadMore = useCallback(async () => {
    // Without the cursor guard, a second "load more" while the first is in
    // flight would append the same page twice.
    if (!cursor || loading) return;
    await load(cursor, true);
  }, [cursor, loading, load]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { jobs, loading, error, hasMore, refresh, loadMore };
}

function describe(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.requiresLogin) return 'Your session has ended. Please sign in again.';
    if (error.retryable) return 'The API is busy or unreachable. Retrying may help.';
    return error.message;
  }
  return 'Could not load jobs.';
}

/**
 * Groups jobs into the operational buckets an operator actually watches.
 *
 * SEARCHING first because a job nobody has taken is the one that needs
 * attention — it is the state where a customer is waiting and nothing is
 * happening on its own.
 */
export function triage(jobs: Job[]): { searching: Job[]; active: Job[]; finished: Job[] } {
  const searching: Job[] = [];
  const active: Job[] = [];
  const finished: Job[] = [];

  for (const job of jobs) {
    switch (job.status) {
      case 'REQUESTED':
      case 'SEARCHING':
        searching.push(job);
        break;
      case 'COMPLETED':
      case 'CANCELLED':
      case 'FAILED':
      case 'EXPIRED':
      case 'DISPUTED':
        finished.push(job);
        break;
      default:
        active.push(job);
    }
  }
  return { searching, active, finished };
}
