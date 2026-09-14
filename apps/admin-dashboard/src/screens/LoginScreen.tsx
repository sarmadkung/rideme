import { useState } from 'react';
import { tokens } from '@platform/ui';
import type { AuthActions, AuthState } from '@platform/mobile/useAuth';

/**
 * Phone-OTP sign-in (document 028) — the same flow every RideMe surface uses,
 * including this one. An admin is a regular user who has been granted the
 * ADMIN or SUPER_ADMIN role; there is no separate admin login mechanism.
 *
 * The flow itself lives in @platform/mobile's useAuth, unchanged: it is pure
 * React with no React Native dependency, so the web dashboard reuses it
 * directly rather than re-implementing phone-OTP a third time.
 */
export function LoginScreen({ auth }: { auth: AuthState & AuthActions }) {
  const [phone, setPhone] = useState('');
  const [code, setCode] = useState('');
  const onPhoneStage = auth.stage === 'phone';

  return (
    <div style={styles.screen}>
      <div style={styles.card}>
        <h1 style={styles.title}>RideMe Admin</h1>
        <p style={styles.subtitle}>
          {onPhoneStage ? 'Enter your phone number to continue.' : `We sent a code to ${auth.phone}.`}
        </p>

        {onPhoneStage ? (
          <input
            style={styles.input}
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="03001234567"
            inputMode="tel"
            autoComplete="tel"
            disabled={auth.pending}
          />
        ) : (
          <input
            style={styles.input}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="6-digit code"
            inputMode="numeric"
            autoComplete="one-time-code"
            disabled={auth.pending}
          />
        )}

        {auth.error !== null && <p style={styles.error}>{auth.error}</p>}

        <button
          style={styles.button}
          disabled={auth.pending}
          onClick={() => {
            if (onPhoneStage) void auth.requestCode(phone.trim());
            else void auth.submitCode(code.trim());
          }}
        >
          {auth.pending ? 'Please wait…' : onPhoneStage ? 'Send code' : 'Verify'}
        </button>

        {!onPhoneStage && (
          <button style={styles.link} onClick={auth.restart} disabled={auth.pending}>
            Use a different number
          </button>
        )}
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  screen: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: tokens.color.background,
    fontFamily: 'system-ui, sans-serif',
  },
  card: { width: 320, padding: tokens.space.lg },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: 600, margin: 0 },
  subtitle: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.sm,
    marginBottom: tokens.space.lg,
  },
  input: {
    width: '100%',
    boxSizing: 'border-box',
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.md,
    color: tokens.color.text,
    fontSize: tokens.fontSize.lg,
    padding: tokens.space.md,
  },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginTop: tokens.space.md },
  button: {
    width: '100%',
    background: tokens.color.accent,
    color: tokens.color.onAccent,
    border: 'none',
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    fontSize: tokens.fontSize.md,
    fontWeight: 600,
    marginTop: tokens.space.lg,
    cursor: 'pointer',
  },
  link: {
    display: 'block',
    width: '100%',
    background: 'none',
    border: 'none',
    color: tokens.color.accent,
    fontSize: tokens.fontSize.sm,
    textAlign: 'center',
    marginTop: tokens.space.md,
    cursor: 'pointer',
  },
};
