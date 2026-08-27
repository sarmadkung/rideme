import { tokens } from '@platform/ui';
import { env } from './env';

/**
 * Placeholder shell. Its only job is to prove the toolchain: TypeScript, React,
 * Vite, workspace package resolution and environment validation. Operational
 * screens arrive with the operations console (Phase 14).
 */
export function App() {
  return (
    <main
      style={{
        minHeight: '100vh',
        background: tokens.color.background,
        color: tokens.color.text,
        fontFamily: 'system-ui, sans-serif',
        padding: tokens.space.xl,
      }}
    >
      <h1 style={{ fontSize: tokens.fontSize.xl, margin: 0 }}>RideMe Admin</h1>
      <p style={{ color: tokens.color.textMuted }}>
        Foundation shell — no operational functionality yet.
      </p>
      <dl style={{ color: tokens.color.textMuted, fontSize: tokens.fontSize.sm }}>
        <dt>Environment</dt>
        <dd data-testid="app-env">{env.appEnv}</dd>
        <dt>API</dt>
        <dd data-testid="api-base-url">{env.apiBaseUrl}</dd>
      </dl>
    </main>
  );
}
