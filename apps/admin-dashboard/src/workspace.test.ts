/**
 * Acceptance criterion 2: all @platform/* packages this app depends on build
 * and are importable from an application. This is the check that proves it.
 */
import { describe, expect, it } from 'vitest';
import { createApiClient } from '@platform/api-client';
import { InMemoryTokenStorage, useAuth } from '@platform/auth';
import { APP_ENVS } from '@platform/config';
import { haversineMeters } from '@platform/maps';
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
