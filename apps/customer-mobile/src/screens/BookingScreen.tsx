import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { fareComponentLabel, formatMoney, tokens } from '@platform/ui';
import type { StopInput } from '@platform/api-client';
import type { BookingActions, BookingState } from '../features/booking/useBooking';

/**
 * Named places, standing in for map selection.
 *
 * There is no map here, and pretending otherwise would be worse than saying
 * so: no map provider is integrated yet (CAP-2), so a customer picks from
 * known points rather than dropping a pin. The flow behind this — quote,
 * confirm, track — is the real one, and swapping this control for a map does
 * not change it.
 */
export const PLACES: Array<{ name: string; stop: StopInput }> = [
  { name: 'Liberty Market', stop: { latitude: 31.5169, longitude: 74.3484 } },
  { name: 'Lahore Airport', stop: { latitude: 31.5216, longitude: 74.4036 } },
  { name: 'Emporium Mall', stop: { latitude: 31.4697, longitude: 74.2728 } },
  { name: 'Anarkali Bazaar', stop: { latitude: 31.5709, longitude: 74.3095 } },
  { name: 'Bahria Town', stop: { latitude: 31.3676, longitude: 74.1836 } },
];

function nameOf(stop: StopInput | null): string | null {
  if (stop === null) return null;
  const match = PLACES.find(
    (place) => place.stop.latitude === stop.latitude && place.stop.longitude === stop.longitude,
  );
  return match?.name ?? `${stop.latitude.toFixed(4)}, ${stop.longitude.toFixed(4)}`;
}

export function BookingScreen({ booking }: { booking: BookingState & BookingActions }) {
  const pickupName = nameOf(booking.pickup);
  const dropoffName = nameOf(booking.dropoff);
  const quote = booking.quote;

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="booking-screen">
      <Text style={styles.title}>Where to?</Text>

      <PlacePicker
        label="Pickup"
        testID="pickup-picker"
        selected={booking.pickup}
        disabled={booking.pending}
        onSelect={booking.setPickup}
      />
      <PlacePicker
        label="Destination"
        testID="dropoff-picker"
        selected={booking.dropoff}
        disabled={booking.pending}
        onSelect={booking.setDropoff}
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
            {pickupName} → {dropoffName}
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
  disabled,
  onSelect,
}: {
  label: string;
  testID: string;
  selected: StopInput | null;
  disabled: boolean;
  onSelect(stop: StopInput | null): void;
}) {
  return (
    <View style={styles.picker} testID={testID}>
      <Text style={styles.pickerLabel}>{label}</Text>
      <View style={styles.chips}>
        {PLACES.map((place) => {
          const active =
            selected?.latitude === place.stop.latitude &&
            selected?.longitude === place.stop.longitude;
          return (
            <Pressable
              key={place.name}
              testID={`${testID}-${place.name}`}
              disabled={disabled}
              style={[styles.chip, active && styles.chipActive]}
              onPress={() => onSelect(active ? null : place.stop)}
            >
              <Text style={[styles.chipText, active && styles.chipTextActive]}>{place.name}</Text>
            </Pressable>
          );
        })}
      </View>
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
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginBottom: tokens.space.md },
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
