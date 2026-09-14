import { useCallback, useMemo, useState } from 'react';
import { StatusBar } from 'expo-status-bar';
import { Pressable, SafeAreaView, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { loadEnvOrNull } from './src/env';
import { getApiClient } from './src/api/client';
import { useAuth, DEV_OTP_BYPASS_CODE } from '@platform/mobile';
import { useBooking } from './src/features/booking/useBooking';
import { useGrocery } from './src/features/grocery/useGrocery';
import { LoginScreen } from './src/screens/LoginScreen';
import { BookingScreen } from './src/screens/BookingScreen';
import { TripScreen } from './src/screens/TripScreen';
import { StoresScreen } from './src/screens/StoresScreen';
import { ShopScreen } from './src/screens/ShopScreen';
import { GroceryCheckoutScreen } from './src/screens/GroceryCheckoutScreen';
import { GroceryOrderScreen } from './src/screens/GroceryOrderScreen';

const env = loadEnvOrNull(process.env as Record<string, string | undefined>);

/**
 * The customer app shell.
 *
 * Which screen is showing follows from state — signed in or not, which service
 * they are using, and how far through it they are — rather than from a
 * navigation stack. There are now two flows to move between, which is what a
 * navigator is for; a two-tab switch is still less machinery than one, and the
 * flows themselves remain linear with no back destination worth preserving.
 * The navigator arrives when there is something to go *back* to.
 */
export default function App() {
  if (env === null) {
    return <NotConfigured />;
  }
  return <Shell />;
}

type Service = 'ride' | 'grocery';

function Shell() {
  const [sessionLost, setSessionLost] = useState(false);
  const [service, setService] = useState<Service>('ride');
  const onSessionExpired = useCallback(() => setSessionLost(true), []);
  const client = useMemo(() => getApiClient({ onSessionExpired }), [onSessionExpired]);

  const auth = useAuth(
    client,
    env?.otpBypass ? { devAutoCode: DEV_OTP_BYPASS_CODE } : undefined,
  );
  const booking = useBooking(client, { city: env?.city });
  const grocery = useGrocery(client);

  if (auth.stage !== 'authenticated' || sessionLost) {
    return (
      <SafeAreaView style={styles.safe} testID="app-root">
        <LoginScreen auth={auth} />
        <StatusBar style="light" />
      </SafeAreaView>
    );
  }

  // Neither flow may be switched away from once it is committed: a ride being
  // tracked and an order being delivered are both things happening in the
  // world, and a tab bar that hid one of them would lose the customer's only
  // way back to it.
  const busy = booking.stage === 'tracking' || grocery.stage === 'tracking';

  return (
    <SafeAreaView style={styles.safe} testID="app-root">
      {!busy && (
        <View style={styles.tabs} testID="service-tabs">
          <Tab label="Ride" active={service === 'ride'} onPress={() => setService('ride')} />
          <Tab
            label="Grocery"
            active={service === 'grocery'}
            onPress={() => setService('grocery')}
          />
        </View>
      )}
      {service === 'ride' || booking.stage === 'tracking' ? (
        <RideFlow booking={booking} client={client} />
      ) : (
        <GroceryFlow grocery={grocery} client={client} />
      )}
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

function RideFlow({
  booking,
  client,
}: {
  booking: ReturnType<typeof useBooking>;
  client: ReturnType<typeof getApiClient>;
}) {
  if (booking.stage === 'tracking') return <TripScreen booking={booking} />;
  return <BookingScreen booking={booking} client={client} />;
}

function GroceryFlow({
  grocery,
  client,
}: {
  grocery: ReturnType<typeof useGrocery>;
  client: ReturnType<typeof getApiClient>;
}) {
  switch (grocery.stage) {
    case 'shopping':
      return <ShopScreen grocery={grocery} />;
    case 'checkout':
      return <GroceryCheckoutScreen grocery={grocery} client={client} />;
    case 'tracking':
      return <GroceryOrderScreen grocery={grocery} />;
    default:
      return <StoresScreen grocery={grocery} client={client} />;
  }
}

function Tab({ label, active, onPress }: { label: string; active: boolean; onPress(): void }) {
  return (
    <Pressable testID={`tab-${label.toLowerCase()}`} style={styles.tab} onPress={onPress}>
      <Text style={active ? styles.tabTextActive : styles.tabText}>{label}</Text>
    </Pressable>
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
  tabs: {
    flexDirection: 'row',
    gap: tokens.space.lg,
    paddingHorizontal: tokens.space.lg,
    paddingTop: tokens.space.md,
  },
  tab: { paddingVertical: tokens.space.sm },
  tabText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  tabTextActive: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
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
