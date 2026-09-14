import { useCallback, useEffect, useState } from 'react';
import type { ApiClient, CreateTariffInput } from '@platform/api-client';
import { ApiError } from '@platform/api-client';
import type { Tariff } from '@platform/types';

/**
 * Pricing configuration (documents 34, 142). A tariff is never edited in
 * place — creating one always adds a new version, which is why this hook has
 * no update/delete: the server's own contract already forbids them.
 */
export interface TariffsState {
  tariffs: Tariff[];
  loading: boolean;
  error: string | null;
  creating: boolean;
  createError: string | null;
}

export interface TariffsActions {
  refresh(): Promise<void>;
  create(input: CreateTariffInput): Promise<boolean>;
}

export function useTariffs(client: ApiClient): TariffsState & TariffsActions {
  const [tariffs, setTariffs] = useState<Tariff[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setTariffs(await client.listTariffs());
    } catch (cause) {
      setError(describe(cause));
    } finally {
      setLoading(false);
    }
  }, [client]);

  const create = useCallback(
    async (input: CreateTariffInput) => {
      setCreating(true);
      setCreateError(null);
      try {
        await client.createTariff(input);
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

  return { tariffs, loading, error, creating, createError, refresh, create };
}

function describe(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.requiresLogin) return 'Your session has ended. Please sign in again.';
    if (error.code === 'forbidden') return 'You do not have permission to manage pricing.';
    if (error.code === 'validation') return Object.values(error.details)[0] ?? error.message;
    return error.message;
  }
  return 'Could not reach the server.';
}
