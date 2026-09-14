import { useCallback, useMemo, useState } from 'react';
import { tokens } from '@platform/ui';
import { useAuth } from '@platform/mobile/useAuth';
import { getApiClient } from './api/client';
import { LoginScreen } from './screens/LoginScreen';
import { ZonesScreen } from './screens/ZonesScreen';
import { PricingScreen } from './screens/PricingScreen';

type Tab = 'zones' | 'pricing';

/**
 * The admin shell.
 *
 * No router: three screens do not justify one, and this mirrors the same
 * "no navigator yet" call the mobile apps already made for a linear flow.
 * Role gating here is UX only — RequireRole on the server is the real guard,
 * so a client that skipped this check would just see every request 403.
 */
export function App() {
  const [sessionLost, setSessionLost] = useState(false);
  const onSessionExpired = useCallback(() => setSessionLost(true), []);
  const client = useMemo(() => getApiClient(onSessionExpired), [onSessionExpired]);
  const auth = useAuth(client);
  const [tab, setTab] = useState<Tab>('zones');

  const signedIn = auth.stage === 'authenticated' && !sessionLost;
  const isAdmin = auth.roles.some((role) => role === 'ADMIN' || role === 'SUPER_ADMIN');

  if (!signedIn) {
    return <LoginScreen auth={auth} />;
  }

  if (!isAdmin) {
    return (
      <main style={styles.page}>
        <p style={styles.notPermitted}>
          This account does not hold the ADMIN or SUPER_ADMIN role. Ask an existing admin to grant
          it.
        </p>
      </main>
    );
  }

  return (
    <main style={styles.page}>
      <h1 style={styles.title}>RideMe Admin</h1>
      <nav style={styles.tabs}>
        {(['zones', 'pricing'] as const).map((t) => (
          <button
            key={t}
            style={{ ...styles.tab, ...(tab === t ? styles.tabActive : {}) }}
            onClick={() => setTab(t)}
          >
            {t === 'zones' ? 'Zones' : 'Pricing'}
          </button>
        ))}
      </nav>
      {tab === 'zones' ? <ZonesScreen client={client} /> : <PricingScreen client={client} />}
    </main>
  );
}

const styles: Record<string, React.CSSProperties> = {
  page: {
    minHeight: '100vh',
    background: tokens.color.background,
    color: tokens.color.text,
    fontFamily: 'system-ui, sans-serif',
    padding: tokens.space.xl,
  },
  title: { fontSize: tokens.fontSize.xl, margin: 0, marginBottom: tokens.space.lg },
  tabs: { display: 'flex', gap: tokens.space.sm, marginBottom: tokens.space.lg },
  tab: {
    background: 'none',
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
    color: tokens.color.textMuted,
    padding: `${tokens.space.sm}px ${tokens.space.md}px`,
    fontSize: tokens.fontSize.sm,
    cursor: 'pointer',
  },
  tabActive: {
    background: tokens.color.surface,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.accent}`,
  },
  notPermitted: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    maxWidth: 480,
  },
};
