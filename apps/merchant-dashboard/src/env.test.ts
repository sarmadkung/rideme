import { describe, expect, it } from 'vitest';
import { ConfigError } from '@platform/config';
import { loadEnv } from './env';

describe('loadEnv', () => {
  it('accepts a complete public environment', () => {
    expect(
      loadEnv({ VITE_APP_ENV: 'development', VITE_API_BASE_URL: 'http://localhost:8080' }),
    ).toEqual({ appEnv: 'development', apiBaseUrl: 'http://localhost:8080' });
  });

  it('fails loudly when a variable is missing', () => {
    expect(() => loadEnv({ VITE_APP_ENV: 'development' })).toThrow(ConfigError);
  });

  it('refuses to start if a secret was given a public prefix', () => {
    expect(() =>
      loadEnv({
        VITE_APP_ENV: 'development',
        VITE_API_BASE_URL: 'http://localhost:8080',
        VITE_JWT_SECRET: 'leaked',
      }),
    ).toThrow(ConfigError);
  });
});
