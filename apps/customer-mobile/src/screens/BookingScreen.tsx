import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { useState } from 'react';
import { fareComponentLabel, formatMoney, tokens } from '@platform/ui';
import type { ApiClient, StopInput } from '@platform/api-client';
import type { Place } from '@platform/types';
import type { BookingActions, BookingState } from '../features/booking/useBooking';
import { usePlaceSearch } from '../features/places/usePlaceSearch';

/**
 * Well-known places, kept as a floor under the search box.
 *
 * Search can be unavailable — no geocoder configured, the provider down, the
 * phone offline — and a customer who cannot type an address should still be
 * able to book. These five are also faster than typing for the trips people
 * actually make most.
 *
 * There is still no map: a customer types or taps, rather than dropping a pin.
 * The flow behind this — quote, confirm, track — is the real one.
 */
export const PLACES: Array<{ name: string; stop: StopInput }> = [
  { name: 'Liberty Market', stop: { latitude: 31.5169, longitude: 74.3484 } },
  { name: 'Lahore Airport', stop: { latitude: 31.5216, longitude: 74.4036 } },
  { name: 'Emporium Mall', stop: { latitude: 31.4697, longitude: 74.2728 } },
  { name: 'Anarkali Bazaar', stop: { latitude: 31.5709, longitude: 74.3095 } },
  { name: 'Bahria Town', stop: { latitude: 31.3676, longitude: 74.1836 } },
];

function describe(stop: StopInput | null): string {
  if (stop === null) return '—';
  return `${stop.latitude.toFixed(4)}, ${stop.longitude.toFixed(4)}`;
}

export function BookingScreen({
  booking,
  client,
  near,
}: {
  booking: BookingState & BookingActions;
  client: ApiClient;
  near?: { latitude: number; longitude: number };
}) {
  // A searched place has a name the platform never stores — StopInput is
  // coordinates. Holding it here keeps the contract unchanged and still lets
  // the quote panel say "Liberty Market" rather than two decimals.
  const [pickupName, setPickupName] = useState<string | null>(null);
  const [dropoffName, setDropoffName] = useState<string | null>(null);
  const quote = booking.quote;

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="booking-screen">
      <Text style={styles.title}>Where to?</Text>

      <PlacePicker
        label="Pickup"
        testID="pickup-picker"
        selected={booking.pickup}
        selectedName={pickupName}
        disabled={booking.pending}
        client={client}
        {...(near ? { near } : {})}
        onSelect={(stop, name) => {
          booking.setPickup(stop);
          setPickupName(name);
        }}
      />
      <PlacePicker
        label="Destination"
        testID="dropoff-picker"
        selected={booking.dropoff}
        selectedName={dropoffName}
        disabled={booking.pending}
        client={client}
        {...(near ? { near } : {})}
        onSelect={(stop, name) => {
          booking.setDropoff(stop);
          setDropoffName(name);
        }}
      />

      {booking.error !== null && (
        <Text style={styles.error} testID="booking-error">
          {booking.error}
        </Text>
      )}

      {quote === null ? (
        <Pressable
          testID="get-quote"
          style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}
          disabled={booking.pending}
          onPress={() => void booking.requestQuote()}
        >
          {booking.pending ? (
            <ActivityIndicator color={tokens.color.text} />
          ) : (
            <Text style={styles.buttonText}>See price</Text>
          )}
        </Pressable>
      ) : (
        <View testID="quote-panel" style={styles.panel}>
          <Text style={styles.panelTitle}>
            {pickupName ?? describe(booking.pickup)} → {dropoffName ?? describe(booking.dropoff)}
          </Text>
          <Text style={styles.meta}>
            {(quote.distance_meters / 1000).toFixed(1)} km ·{' '}
            {Math.round(quote.duration_seconds / 60)} min
          </Text>

          {/* Document 034 requires the full breakdown, and BD-02's demand line
              can now move a fare — so a customer must be able to see which
              line did it rather than only the figure it produced. */}
          {quote.lines.map((line, index) => (
            <View key={`${line.component}-${index}`} style={styles.row}>
              <Text style={styles.rowLabel}>{fareComponentLabel(line.component)}</Text>
              <Text style={styles.rowValue}>{formatMoney(line.amount)}</Text>
            </View>
          ))}

          <View style={[styles.row, styles.totalRow]}>
            <Text style={styles.totalLabel}>Total</Text>
            <Text style={styles.totalValue} testID="quote-total">
              {formatMoney(quote.total)}
            </Text>
          </View>

          {/* Document 096 forbids presenting an estimate as exact. */}
          {quote.route_confidence !== 'MEASURED' && (
            <Text style={styles.disclaimer} testID="route-disclaimer">
              This route is estimated, so the final fare may differ.
            </Text>
          )}

          <Pressable
            testID="confirm-booking"
            style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}
            disabled={booking.pending}
            onPress={() => void booking.confirm()}
          >
            {booking.pending ? (
              <ActivityIndicator color={tokens.color.text} />
            ) : (
              <Text style={styles.buttonText}>Confirm ride</Text>
            )}
          </Pressable>
        </View>
      )}
    </ScrollView>
  );
}

function PlacePicker({
  label,
  testID,
  selected,
  selectedName,
  disabled,
  client,
  near,
  onSelect,
}: {
  label: string;
  testID: string;
  selected: StopInput | null;
  selectedName: string | null;
  disabled: boolean;
  client: ApiClient;
  near?: { latitude: number; longitude: number };
  onSelect(stop: StopInput | null, name: string | null): void;
}) {
  const search = usePlaceSearch(client, near);

  const choose = (stop: StopInput, name: string) => {
    onSelect(stop, name);
    search.clear();
  };

  return (
    <View style={styles.picker} testID={testID}>
      <Text style={styles.pickerLabel}>{label}</Text>

      {selected !== null ? (
        <Pressable
          testID={`${testID}-clear`}
          disabled={disabled}
          style={styles.selected}
          onPress={() => onSelect(null, null)}
        >
          <Text style={styles.selectedName}>{selectedName ?? 'Selected'}</Text>
          <Text style={styles.selectedChange}>Change</Text>
        </Pressable>
      ) : (
        <>
          <TextInput
            testID={`${testID}-input`}
            style={styles.input}
            value={search.query}
            editable={!disabled}
            placeholder="Search for a place or address"
            placeholderTextColor={tokens.color.textMuted}
            autoCorrect={false}
            onChangeText={search.setQuery}
          />

          {search.stage === 'searching' && (
            <ActivityIndicator testID={`${testID}-searching`} color={tokens.color.textMuted} />
          )}

          {/* An outage and an empty result must not read the same, or a
              customer retypes their address until they give up. */}
          {search.stage === 'failed' && (
            <Text style={styles.searchNotice} testID={`${testID}-search-error`}>
              {search.error}
            </Text>
          )}
          {search.stage === 'done' && search.results.length === 0 && (
            <Text style={styles.searchNotice} testID={`${testID}-no-results`}>
              No places matched. Try a different name.
            </Text>
          )}

          {search.results.map((place: Place, index: number) => (
            <Pressable
              key={`${place.name}-${index}`}
              testID={`${testID}-result-${index}`}
              disabled={disabled}
              style={styles.result}
              onPress={() =>
                choose({ latitude: place.point.lat, longitude: place.point.lon }, place.name)
              }
            >
              <Text style={styles.resultName}>{place.name}</Text>
              <Text style={styles.resultAddress} numberOfLines={1}>
                {place.address}
              </Text>
            </Pressable>
          ))}

          {/* The floor: always reachable, whatever search is doing. */}
          {search.results.length === 0 && (
            <View style={styles.chips}>
              {PLACES.map((place) => (
                <Pressable
                  key={place.name}
                  testID={`${testID}-${place.name}`}
                  disabled={disabled}
                  style={styles.chip}
                  onPress={() => choose(place.stop, place.name)}
                >
                  <Text style={styles.chipText}>{place.name}</Text>
                </Pressable>
              ))}
            </View>
          )}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { backgroundColor: tokens.color.background, padding: tokens.space.lg, flexGrow: 1 },
  title: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.xl,
    fontWeight: '600',
    marginBottom: tokens.space.lg,
  },
  picker: { marginBottom: tokens.space.lg },
  pickerLabel: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.sm,
  },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: tokens.space.sm },
  chip: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.lg,
    paddingVertical: tokens.space.sm,
    paddingHorizontal: tokens.space.md,
  },
  chipActive: { backgroundColor: tokens.color.accent, borderColor: tokens.color.accent },
  input: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    color: tokens.color.text,
    fontSize: tokens.fontSize.md,
    padding: tokens.space.sm,
    marginBottom: tokens.space.sm,
  },
  selected: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.sm,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  selectedName: { color: tokens.color.text, fontSize: tokens.fontSize.md, flexShrink: 1 },
  selectedChange: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  result: {
    paddingVertical: tokens.space.sm,
    borderBottomColor: tokens.color.border,
    borderBottomWidth: 1,
  },
  resultName: { color: tokens.color.text, fontSize: tokens.fontSize.md },
  resultAddress: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  searchNotice: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.sm,
  },
  chipText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  chipTextActive: { color: tokens.color.text, fontWeight: '600' },
  panel: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
  },
  panelTitle: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  meta: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.xs,
    marginBottom: tokens.space.md,
  },
  row: { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: tokens.space.xs },
  rowLabel: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  rowValue: { color: tokens.color.text, fontSize: tokens.fontSize.sm },
  totalRow: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    marginTop: tokens.space.sm,
    paddingTop: tokens.space.sm,
  },
  totalLabel: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  totalValue: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  disclaimer: {
    color: tokens.color.warning,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.sm,
  },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.md,
  },
  buttonPressed: { opacity: 0.8 },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
});
