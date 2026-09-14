import { useCallback, useState } from 'react';
import { getApiClient, resetApiClient } from './api/client';
import { LoginScreen } from './screens/LoginScreen';
import { OrdersScreen } from './screens/OrdersScreen';

/**
 * The merchant console (documents 72, 77).
 *
 * There is no router. Document 77 names React Router, TanStack Query, Zustand
 * and Tailwind; this slice adds none of them, for the reason ADR-011 records —
 * two views and one server-state hook do not need four libraries, and the
 * workspace has none of them yet. The screens are deliberately shaped so a
 * router can be dropped in when there is a third view to route to.
 */
export function App() {
  const [signedIn, setSignedIn] = useState(false);
  const onSessionExpired = useCallback(() => {
    resetApiClient();
    setSignedIn(false);
  }, []);
  const client = getApiClient(onSessionExpired);

  if (!signedIn) {
    return <LoginScreen client={client} onSignedIn={() => setSignedIn(true)} />;
  }
  return <OrdersScreen client={client} />;
}
