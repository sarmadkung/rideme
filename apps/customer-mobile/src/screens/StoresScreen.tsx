import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useEffect, useState } from 'react';
import { tokens } from '@platform/ui';
import type { ApiClient, StopInput } from '@platform/api-client';
import type { Store } from '@platform/types';
import { PLACES, PlacePicker } from '../components/PlacePicker';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

/**
 * Where the shops are searched from, when nothing better is known.
 *
 * The app has no location permission — foreground location is the driver's
 * feature — so "near me" has to be asked rather than sensed. Starting from a
 * known point means the first screen has shops on it instead of an empty state
 * asking the customer to do something before anything happens.
 */
const DEFAULT_NEAR = PLACES[0] as { name: string; stop: StopInput };

export function StoresScreen({
  grocery,
  client,
}: {
  grocery: GroceryState & GroceryActions;
  client: ApiClient;
}) {
  const [near, setNear] = useState<StopInput>(DEFAULT_NEAR.stop);
  const [nearName, setNearName] = useState<string>(DEFAULT_NEAR.name);

  // Named rather than reached through `grocery`, which is a fresh object on
  // every render: the action itself is stable, so the search re-runs when the
  // position changes and not once more.
  const { findStores } = grocery;
  useEffect(() => {
    void findStores({ latitude: near.latitude, longitude: near.longitude });
  }, [findStores, near.latitude, near.longitude]);

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="stores-screen">
      <Text style={styles.title}>Shops near {nearName}</Text>

      <PlacePicker
        label="Look near"
        testID="stores-near"
        selected={null}
        selectedName={null}
        disabled={grocery.pending}
        client={client}
        near={{ latitude: near.latitude, longitude: near.longitude }}
        placeholder="Search another area"
        onSelect={(stop, name) => {
          if (stop === null) return;
          setNear(stop);
          setNearName(name ?? 'here');
        }}
      />

      {grocery.error !== null && (
        <Text style={styles.error} testID="stores-error">
          {grocery.error}
        </Text>
      )}

      {grocery.pending && <ActivityIndicator testID="stores-loading" color={tokens.color.accent} />}

      {!grocery.pending && grocery.stores.length === 0 && grocery.error === null && (
        <Text style={styles.empty} testID="stores-empty">
          No shops deliver here yet.
        </Text>
      )}

      {grocery.stores.map((store) => (
        <StoreRow
          key={store.id}
          store={store}
          disabled={grocery.pending}
          onPress={() => void grocery.enterStore(store)}
        />
      ))}
    </ScrollView>
  );
}

function StoreRow({
  store,
  disabled,
  onPress,
}: {
  store: Store;
  disabled: boolean;
  onPress(): void;
}) {
  return (
    <Pressable
      testID={`store-${store.id}`}
      style={styles.row}
      // A shut shop is listed and not tappable. Hiding it would tell a
      // customer their usual kiryana had vanished; letting them fill a cart in
      // it would waste their time and end in a rejection.
      disabled={disabled || !store.open}
      onPress={onPress}
    >
      <View style={styles.rowText}>
        <Text style={styles.storeName}>{store.merchant_name}</Text>
        <Text style={styles.storeMeta} numberOfLines={1}>
          {store.name} · {formatDistance(store.distance_m)}
        </Text>
      </View>
      <Text style={store.open ? styles.open : styles.closed}>{store.open ? 'Open' : 'Closed'}</Text>
    </Pressable>
  );
}

/** Metres under a kilometre, one decimal above it. Nobody walks 1,240 m. */
export function formatDistance(metres: number): string {
  if (metres < 1000) return `${Math.round(metres)} m`;
  return `${(metres / 1000).toFixed(1)} km`;
}

const styles = StyleSheet.create({
  screen: { backgroundColor: tokens.color.background, padding: tokens.space.lg, flexGrow: 1 },
  title: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.xl,
    fontWeight: '600',
    marginBottom: tokens.space.lg,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    marginBottom: tokens.space.sm,
  },
  rowText: { flexShrink: 1 },
  storeName: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  storeMeta: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  open: { color: tokens.color.success, fontSize: tokens.fontSize.sm },
  closed: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  empty: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
});
