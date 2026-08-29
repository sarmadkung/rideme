import { useCallback, useMemo, useState } from 'react';
import { StatusBar } from 'expo-status-bar';
import { SafeAreaView, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { loadEnvOrNull } from './src/env';
import { getApiClient } from './src/api/client';
import { useAuth } from '@platform/mobile';
import { useBooking } from './src/features/booking/useBooking';
import { LoginScreen } from './src/screens/LoginScreen';
import { BookingScreen } from './src/screens/BookingScreen';
import { TripScreen } from './src/screens/TripScreen';

const env = loadEnvOrNull(process.env as Record<string, string | undefined>);

/**
 * The customer app shell.
 *
 * Which screen is showing follows from state — signed in or not, and whether a
 * ride is in flight — rather than from a navigation stack. There is no
 * navigator yet: this flow is linear and has no back destination worth
 * preserving, and introducing one before there is a second flow to move
 * between would be scaffolding without a user.
 */
export default function App() {
  if (env === null) {
    return <NotConfigured />;
  }
  return <Shell />;
}

function Shell() {
  const [sessionLost, setSessionLost] = useState(false);
  const onSessionExpired = useCallback(() => setSessionLost(true), []);
  const client = useMemo(() => getApiClient({ onSessionExpired }), [onSessionExpired]);

  const auth = useAuth(client);
  const booking = useBooking(client, { city: env?.city });

  if (auth.stage !== 'authenticated' || sessionLost) {
    return (
      <SafeAreaView style={styles.safe} testID="app-root">
        <LoginScreen auth={auth} />
        <StatusBar style="light" />
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safe} testID="app-root">
      {booking.stage === 'tracking' ? (
        <TripScreen booking={booking} />
      ) : (
        <BookingScreen booking={booking} />
      )}
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

/**
 * Shown when the app has no API base URL.
 *
 * Failing visibly beats starting a shell that cannot reach anything: every
 * screen behind it would show a network error and none would say why.
 */
function NotConfigured() {
  return (
    <View style={styles.centred} testID="app-root">
      <Text style={styles.title}>RideMe</Text>
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
