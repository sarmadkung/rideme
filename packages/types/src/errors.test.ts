import { describe, expect, it } from 'vitest';
import { ERROR_CODES, isErrorCode } from './errors.js';

describe('error taxonomy', () => {
  it('matches the six documented codes plus internal', () => {
    expect(ERROR_CODES).toEqual([
      'not_found',
      'unauthorized',
      'forbidden',
      'conflict',
      'validation',
      'unavailable',
      'internal',
    ]);
  });

  it('narrows unknown values', () => {
    expect(isErrorCode('conflict')).toBe(true);
    expect(isErrorCode('teapot')).toBe(false);
    expect(isErrorCode(undefined)).toBe(false);
  });
});
