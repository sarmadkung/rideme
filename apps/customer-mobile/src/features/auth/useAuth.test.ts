import { act, renderHook, waitFor } from '@testing-library/react-native';
import { ApiError } from '@platform/api-client';
import { messageFor, useAuth } from '@platform/mobile';

function stubClient(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    requestOtp: jest.fn(async () => ({ expiresAt: new Date('2026-08-28T12:05:00Z') })),
    verifyOtp: jest.fn(async () => ({
      accessToken: 'a',
      expiresAt: new Date('2026-08-28T12:05:00Z'),
    })),
    ...overrides,
  } as never;
}

describe('useAuth', () => {
  it('walks the documented phone → code → authenticated flow', async () => {
    const client = stubClient();
    const { result } = renderHook(() => useAuth(client));

    expect(result.current.stage).toBe('phone');

    await act(async () => {
      await result.current.requestCode('03001234567');
    });
    expect(result.current.stage).toBe('code');
    expect(result.current.phone).toBe('03001234567');
    expect(result.current.codeExpiresAt).not.toBeNull();

    await act(async () => {
      await result.current.submitCode('123456');
    });
    await waitFor(() => expect(result.current.stage).toBe('authenticated'));
  });

  it('keeps the user on the code step when the code is wrong', async () => {
    // Sending them back to the phone step would make them retype the number
    // and request a second code, invalidating the one they are holding.
    const client = stubClient({
      verifyOtp: jest.fn(async () => {
        throw new ApiError(401, { code: 'unauthorized', message: 'no', request_id: 'r' });
      }),
    });
    const { result } = renderHook(() => useAuth(client));

    await act(async () => {
      await result.current.requestCode('03001234567');
    });
    await act(async () => {
      const ok = await result.current.submitCode('000000');
      expect(ok).toBe(false);
    });

    expect(result.current.stage).toBe('code');
    expect(result.current.error).toContain('incorrect');
    expect(result.current.pending).toBe(false);
  });

  it('surfaces rate limiting as advice rather than an error code', async () => {
    const client = stubClient({
      requestOtp: jest.fn(async () => {
        throw new ApiError(429, { code: 'rate_limited', message: 'slow down', request_id: 'r' });
      }),
    });
    const { result } = renderHook(() => useAuth(client));

    await act(async () => {
      await result.current.requestCode('03001234567');
    });
    expect(result.current.stage).toBe('phone');
    expect(result.current.error).toMatch(/wait a few minutes/i);
  });

  it('clears the error when a new attempt starts', async () => {
    // A stale error under a fresh attempt reads as though the new attempt
    // failed too.
    let shouldFail = true;
    const client = stubClient({
      requestOtp: jest.fn(async () => {
        if (shouldFail) {
          shouldFail = false;
          throw new ApiError(429, { code: 'rate_limited', message: 'slow', request_id: 'r' });
        }
        return { expiresAt: new Date('2026-08-28T12:05:00Z') };
      }),
    });
    const { result } = renderHook(() => useAuth(client));

    await act(async () => {
      await result.current.requestCode('03001234567');
    });
    expect(result.current.error).not.toBeNull();

    await act(async () => {
      await result.current.requestCode('03001234567');
    });
    expect(result.current.error).toBeNull();
    expect(result.current.stage).toBe('code');
  });
});

describe('messageFor', () => {
  it('never leaks an internal code to the user', () => {
    for (const code of [
      'not_found',
      'conflict',
      'internal',
      'rate_limited',
      'unauthorized',
    ] as const) {
      const message = messageFor(new ApiError(400, { code, message: 'raw', request_id: 'r' }));
      expect(message).not.toContain('_');
      expect(message).not.toContain(code);
    }
  });

  it('handles a non-API error without exposing it', () => {
    expect(messageFor(new Error('connection reset by peer at 10.0.0.1'))).not.toContain('10.0.0.1');
  });
});
