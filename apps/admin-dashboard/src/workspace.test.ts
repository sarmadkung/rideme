/**
 * Acceptance criterion 2: all @platform/* packages this app depends on build
 * and are importable from an application. This is the check that proves it.
 */
import { describe, expect, it } from 'vitest';
import { createApiClient } from '@platform/api-client';
import { InMemoryTokenStorage } from '@platform/auth';
import { APP_ENVS } from '@platform/config';
import { haversineMeters } from '@platform/maps';
// The /useAuth subpath, not the package root: the root also re-exports
// tokenStorage.ts, which imports expo-secure-store — a native-only module
// this web app's bundler must never be asked to resolve.
import { useAuth } from '@platform/mobile/useAuth';
import { ERROR_CODES } from '@platform/types';
import { tokens } from '@platform/ui';
import { healthResponseSchema } from '@platform/validation';

describe('@platform workspace packages', () => {
  it('every package this app depends on resolves and exports usable values', () => {
    expect(typeof createApiClient).toBe('function');
    expect(new InMemoryTokenStorage()).toBeInstanceOf(InMemoryTokenStorage);
    expect(APP_ENVS).toContain('production');
    expect(haversineMeters({ lat: 0, lng: 0 }, { lat: 0, lng: 0 })).toBe(0);
    expect(typeof useAuth).toBe('function');
    expect(ERROR_CODES).toContain('validation');
    expect(tokens.space.md).toBe(16);
    expect(healthResponseSchema.safeParse({}).success).toBe(false);
  });
});
