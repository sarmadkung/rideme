import { useCallback, useEffect, useState } from 'react';
import type { ApiClient, CreateZoneInput } from '@platform/api-client';
import { ApiError } from '@platform/api-client';
import type { Zone } from '@platform/types';

/**
 * Service zones (document 97): the geographic circles pricing keys off. The
 * server enforces who may create one — this hook just surfaces what it says.
 */
export interface ZonesState {
  zones: Zone[];
  loading: boolean;
  error: string | null;
  creating: boolean;
  createError: string | null;
}

export interface ZonesActions {
  refresh(): Promise<void>;
  create(input: CreateZoneInput): Promise<boolean>;
}

export function useZones(client: ApiClient): ZonesState & ZonesActions {
  const [zones, setZones] = useState<Zone[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setZones(await client.listZones());
    } catch (cause) {
      setError(describe(cause));
    } finally {
      setLoading(false);
    }
  }, [client]);

  const create = useCallback(
    async (input: CreateZoneInput) => {
      setCreating(true);
      setCreateError(null);
      try {
        await client.createZone(input);
        await refresh();
        return true;
      } catch (cause) {
        setCreateError(describe(cause));
        return false;
      } finally {
        setCreating(false);
      }
    },
    [client, refresh],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { zones, loading, error, creating, createError, refresh, create };
}

function describe(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.requiresLogin) return 'Your session has ended. Please sign in again.';
    if (error.code === 'forbidden') return 'You do not have permission to manage zones.';
    if (error.code === 'validation') return Object.values(error.details)[0] ?? error.message;
    return error.message;
  }
  return 'Could not reach the server.';
}
