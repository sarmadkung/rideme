import { ConfigError } from '@platform/config';
import { loadEnv, loadEnvOrNull } from './env';

describe('loadEnv', () => {
  it('accepts a complete public environment', () => {
    expect(
      loadEnv({
        EXPO_PUBLIC_APP_ENV: 'development',
        EXPO_PUBLIC_API_BASE_URL: 'http://localhost:8080',
      }),
    ).toEqual({ appEnv: 'development', apiBaseUrl: 'http://localhost:8080' });
  });

  it('fails when a variable is missing', () => {
    expect(() => loadEnv({ EXPO_PUBLIC_APP_ENV: 'development' })).toThrow(ConfigError);
  });

  it('refuses a secret carrying a public prefix', () => {
    expect(() =>
      loadEnv({
        EXPO_PUBLIC_APP_ENV: 'development',
        EXPO_PUBLIC_API_BASE_URL: 'http://localhost:8080',
        EXPO_PUBLIC_JWT_SECRET: 'leaked',
      }),
    ).toThrow(ConfigError);
  });

  it('degrades to null rather than crashing the app shell', () => {
    expect(loadEnvOrNull({})).toBeNull();
  });
});
