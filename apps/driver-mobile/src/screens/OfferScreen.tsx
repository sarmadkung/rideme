import { Pressable, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import type { JobStop } from '@platform/types';
import type { ShiftActions, ShiftState } from '../features/shift/useShift';

function stopOf(stops: JobStop[], type: string): JobStop | undefined {
  return stops.find((stop) => stop.type === type);
}

function describe(stop: JobStop | undefined): string {
  if (stop === undefined) return 'Unknown';
  if (stop.address !== undefined && stop.address !== '') return stop.address;
  return `${stop.latitude.toFixed(4)}, ${stop.longitude.toFixed(4)}`;
}

/**
 * An offer, with the countdown the driver is racing.
 *
 * The countdown comes from the server's expiry rather than a local timer, so
 * the driver is never shown time they do not have. At zero the buttons go:
 * tapping Accept on a lapsed offer would fail, and a button that always fails
 * is worse than no button.
 */
export function OfferScreen({ shift }: { shift: ShiftState & ShiftActions }) {
  const assignment = shift.assignment;
  if (assignment === null) return null;

  const job = assignment.job;
  const secondsLeft = shift.offerSecondsLeft;
  const lapsed = secondsLeft !== null && secondsLeft <= 0;

  return (
    <View style={styles.screen} testID="offer-screen">
      <Text style={styles.title}>New job</Text>
      {secondsLeft !== null && (
        <Text style={styles.countdown} testID="offer-countdown">
          {lapsed ? 'Offer expired' : `${secondsLeft}s to respond`}
        </Text>
      )}

      <View style={styles.card}>
        <Text style={styles.label}>Pickup</Text>
        <Text style={styles.value} testID="offer-pickup">
          {describe(stopOf(job.stops, 'PICKUP'))}
        </Text>
        <Text style={[styles.label, styles.spaced]}>Destination</Text>
        <Text style={styles.value} testID="offer-dropoff">
          {describe(stopOf(job.stops, 'DROPOFF'))}
        </Text>
      </View>

      {shift.error !== null && (
        <Text style={styles.error} testID="offer-error">
          {shift.error}
        </Text>
      )}

      <View style={styles.actions}>
        <Pressable
          testID="offer-reject"
          style={({ pressed }) => [styles.rejectButton, pressed && styles.pressed]}
          disabled={shift.pending || lapsed}
          onPress={() => void shift.reject()}
        >
          <Text style={styles.rejectText}>Decline</Text>
        </Pressable>
        <Pressable
          testID="offer-accept"
          style={({ pressed }) => [styles.acceptButton, pressed && styles.pressed]}
          disabled={shift.pending || lapsed}
          onPress={() => void shift.accept()}
        >
          <Text style={styles.buttonText}>Accept</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: tokens.color.background,
    padding: tokens.space.lg,
    justifyContent: 'center',
  },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: '600' },
  countdown: {
    color: tokens.color.warning,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.xs,
    marginBottom: tokens.space.md,
  },
  card: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
  },
  label: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  value: { color: tokens.color.text, fontSize: tokens.fontSize.md, marginTop: tokens.space.xs },
  spaced: { marginTop: tokens.space.md },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginTop: tokens.space.md },
  actions: { flexDirection: 'row', gap: tokens.space.md, marginTop: tokens.space.lg },
  acceptButton: {
    flex: 2,
    backgroundColor: tokens.color.success,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  rejectButton: {
    flex: 1,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  rejectText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  pressed: { opacity: 0.8 },
});
