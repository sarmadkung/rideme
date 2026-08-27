import { describe, expect, it } from 'vitest';
import {
  ConfigError,
  assertNoSecretsInPublicEnv,
  isPublicEnvKey,
  requireAppEnv,
  requireEnv,
} from './index.js';

describe('requireEnv', () => {
  it('returns a present value', () => {
    expect(requireEnv({ API_BASE_URL: 'http://localhost:8080' }, 'API_BASE_URL')).toBe(
      'http://localhost:8080',
    );
  });

  it('throws on missing or blank', () => {
    expect(() => requireEnv({}, 'API_BASE_URL')).toThrow(ConfigError);
    expect(() => requireEnv({ API_BASE_URL: '   ' }, 'API_BASE_URL')).toThrow(ConfigError);
  });
});

describe('requireAppEnv', () => {
  it('accepts documented environments and rejects others', () => {
    expect(requireAppEnv({ APP_ENV: 'staging' }, 'APP_ENV')).toBe('staging');
    expect(() => requireAppEnv({ APP_ENV: 'prod' }, 'APP_ENV')).toThrow(ConfigError);
  });
});

describe('public/secret separation', () => {
  it('recognises public prefixes', () => {
    expect(isPublicEnvKey('VITE_API_BASE_URL')).toBe(true);
    expect(isPublicEnvKey('EXPO_PUBLIC_API_BASE_URL')).toBe(true);
    expect(isPublicEnvKey('JWT_SECRET')).toBe(false);
  });

  it('rejects a secret that was given a public prefix', () => {
    expect(() => assertNoSecretsInPublicEnv({ VITE_JWT_SECRET: 'x' })).toThrow(ConfigError);
    expect(() => assertNoSecretsInPublicEnv({ JWT_SECRET: 'x' })).not.toThrow();
  });
});
