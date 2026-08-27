import { describe, expect, it } from 'vitest';
import { cn, tokens } from './index.js';

describe('cn', () => {
  it('drops falsy values', () => {
    expect(cn('a', false, undefined, 'b', null, '')).toBe('a b');
  });
});

describe('tokens', () => {
  it('exposes a complete colour scale', () => {
    expect(Object.keys(tokens.color)).toContain('danger');
    expect(tokens.space.md).toBe(16);
  });
});
