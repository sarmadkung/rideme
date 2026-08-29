import { useCallback, useEffect, useMemo, useState } from 'react';
import { StatusBar } from 'expo-status-bar';
import { SafeAreaView, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { useAuth } from '@platform/mobile';
import { loadEnvOrNull } from './src/env';
import { getApiClient } from './src/api/client';
import { useShift, isOffer } from './src/features/shift/useShift';
import { LoginScreen } from './src/screens/LoginScreen';
import { ShiftScreen } from './src/screens/ShiftScreen';
import { OfferScreen } from './src/screens/OfferScreen';
import { TripScreen } from './src/screens/TripScreen';

const env = loadEnvOrNull(process.env as Record<string, string | undefined>);

/**
 * The driver app shell.
 *
 * Which screen shows follows from what the server says the driver is holding:
 * an unanswered offer, a trip in progress, or neither. Deciding that locally
 * would let the app disagree with the server about whether a job is still
 * available.
 */
export default function App() {
  if (env === null) return <NotConfigured />;
  return <Shell />;
}

function Shell() {
  const [sessionLost, setSessionLost] = useState(false);
  const onSessionExpired = useCallback(() => setSessionLost(true), []);
  const client = useMemo(() => getApiClient({ onSessionExpired }), [onSessionExpired]);

  const auth = useAuth(client);
  const shift = useShift(client);
  const signedIn = auth.stage === 'authenticated' && !sessionLost;

  // Load the driver's real state as soon as they are signed in. Without this
  // the app would show "offline" to a driver who is mid-trip — the server
  // knows, the app just never asked.
  const { refresh } = shift;
  useEffect(() => {
    if (signedIn) void refresh();
  }, [signedIn, refresh]);

  if (!signedIn) {
    return (
      <SafeAreaView style={styles.safe} testID="app-root">
        <LoginScreen auth={auth} />
        <StatusBar style="light" />
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safe} testID="app-root">
      {isOffer(shift.assignment) ? (
        <OfferScreen shift={shift} />
      ) : shift.assignment !== null ? (
        <TripScreen shift={shift} />
      ) : (
        <ShiftScreen shift={shift} position={DEFAULT_POSITION} />
      )}
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

/**
 * Where the driver is, pending a real location source.
 *
 * `expo-location` is not wired up yet, so going online reports a fixed point.
 * This is the one honest placeholder in the flow and it is marked as such: a
 * driver going online from the wrong coordinates would be offered jobs across
 * the city, so this must be replaced before the app is put in front of anyone.
 */
const DEFAULT_POSITION = { latitude: 31.5204, longitude: 74.3587 };

function NotConfigured() {
  return (
    <View style={styles.centred} testID="app-root">
      <Text style={styles.title}>RideMe Driver</Text>
      <Text style={styles.subtitle} testID="app-env">
        environment not configured
      </Text>
      <StatusBar style="light" />
    </View>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: tokens.color.background },
  centred: {
    flex: 1,
    backgroundColor: tokens.color.background,
    alignItems: 'center',
    justifyContent: 'center',
    padding: tokens.space.lg,
  },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: '600' },
  subtitle: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.sm,
  },
});
