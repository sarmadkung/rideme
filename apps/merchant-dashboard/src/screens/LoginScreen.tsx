import { useState, type CSSProperties, type FormEvent } from 'react';
import type { ApiClient } from '@platform/api-client';
import { useAuth } from '@platform/auth';
import { tokens } from '@platform/ui';

/**
 * Phone then code, the same flow the mobile apps use (document 28).
 *
 * The flow itself is `@platform/auth`'s: one implementation for every surface,
 * so a shop and a driver meet the same rules and the same messages. This file
 * is only the web rendering of it.
 */
export function LoginScreen({ client, onSignedIn }: { client: ApiClient; onSignedIn: () => void }) {
  const auth = useAuth(client);
  const [value, setValue] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (auth.stage === 'phone') {
      await auth.requestCode(value.trim());
      setValue('');
      return;
    }
    if (await auth.submitCode(value.trim())) onSignedIn();
  }

  const onCode = auth.stage === 'code';

  return (
    <main style={styles.page}>
      <form style={styles.card} onSubmit={(event) => void submit(event)}>
        <h1 style={styles.heading}>RideMe Merchant</h1>
        <p style={styles.muted}>
          {onCode ? `We sent a code to ${auth.phone}.` : 'Sign in to your shop.'}
        </p>

        <label style={styles.label} htmlFor="credential">
          {onCode ? 'Code' : 'Phone number'}
        </label>
        <input
          id="credential"
          style={styles.input}
          value={value}
          onChange={(event) => setValue(event.target.value)}
          // A phone keypad on a counter tablet, and no autocorrect on a code.
          inputMode="numeric"
          autoComplete={onCode ? 'one-time-code' : 'tel'}
          disabled={auth.pending}
        />

        {auth.error ? (
          <p role="alert" style={styles.error}>
            {auth.error}
          </p>
        ) : null}

        <button style={styles.primary} type="submit" disabled={auth.pending || value.trim() === ''}>
          {auth.pending ? 'Please wait…' : onCode ? 'Sign in' : 'Send code'}
        </button>

        {onCode ? (
          <button
            style={styles.link}
            type="button"
            onClick={() => {
              auth.restart();
              setValue('');
            }}
          >
            Use a different number
          </button>
        ) : null}
      </form>
    </main>
  );
}

const styles: Record<string, CSSProperties> = {
  page: {
    minHeight: '100vh',
    display: 'grid',
    placeItems: 'center',
    background: tokens.color.background,
    color: tokens.color.text,
    fontFamily: 'system-ui, sans-serif',
  },
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: tokens.space.sm,
    width: 320,
    padding: tokens.space.lg,
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.lg,
  },
  heading: { fontSize: tokens.fontSize.lg, margin: 0 },
  muted: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, margin: 0 },
  label: { fontSize: tokens.fontSize.sm, color: tokens.color.textMuted },
  input: {
    padding: tokens.space.sm,
    fontSize: tokens.fontSize.md,
    background: tokens.color.background,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
  },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, margin: 0 },
  primary: {
    padding: tokens.space.sm,
    fontSize: tokens.fontSize.md,
    background: tokens.color.accent,
    color: tokens.color.text,
    border: 'none',
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
  },
  link: {
    background: 'none',
    border: 'none',
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    cursor: 'pointer',
  },
};
