import { describe, expect, it } from 'vitest';
import { InMemoryTokenStorage, isExpired } from './index.js';

const tokens = { accessToken: 'a', refreshToken: 'r', expiresAt: 1_000_000 };

describe('InMemoryTokenStorage', () => {
  it('round-trips and clears', async () => {
    const storage = new InMemoryTokenStorage();
    expect(await storage.read()).toBeNull();
    await storage.write(tokens);
    expect(await storage.read()).toEqual(tokens);
    await storage.clear();
    expect(await storage.read()).toBeNull();
  });
});

describe('isExpired', () => {
  it('treats the clock-skew window as already expired', () => {
    expect(isExpired(tokens, 900_000)).toBe(false);
    expect(isExpired(tokens, 999_990)).toBe(true);
    expect(isExpired(tokens, 1_000_001)).toBe(true);
  });
});
