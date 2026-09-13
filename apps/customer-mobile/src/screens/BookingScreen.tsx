import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useState } from 'react';
import { fareComponentLabel, formatMoney, tokens } from '@platform/ui';
import type { ApiClient, StopInput } from '@platform/api-client';
import type { BookingActions, BookingState } from '../features/booking/useBooking';
// The picker moved to `components/` when grocery checkout became its second
// consumer. `PLACES` is re-exported because this screen's tests name it.
import { PLACES, PlacePicker } from '../components/PlacePicker';

export { PLACES };

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
