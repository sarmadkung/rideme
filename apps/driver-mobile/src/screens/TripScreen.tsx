import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import type { JobStop } from '@platform/types';
import { nextCommand, type ShiftActions, type ShiftState } from '../features/shift/useShift';

const STAGE: Record<string, string> = {
  ACCEPTED: 'Head to the pickup',
  ARRIVING: 'Head to the pickup',
  AT_PICKUP: 'Waiting for the customer',
  IN_PROGRESS: 'On the trip',
  AT_DROPOFF: 'At the destination',
  COMPLETED: 'Trip complete',
  CANCELLED: 'The customer cancelled',
};

function stopOf(stops: JobStop[], type: string): JobStop | undefined {
  return stops.find((stop) => stop.type === type);
}

function describe(stop: JobStop | undefined): string {
  if (stop === undefined) return 'Unknown';
  if (stop.address !== undefined && stop.address !== '') return stop.address;
  return `${stop.latitude.toFixed(4)}, ${stop.longitude.toFixed(4)}`;
}

/**
 * The trip in progress, and the one command available at this point in it.
 *
 * Document 035's sequence is walked a step at a time. Showing every command at
 * once would let a driver complete a trip they never started, and the server
 * would refuse — leaving the driver tapping a button that does nothing.
 */
export function TripScreen({ shift }: { shift: ShiftState & ShiftActions }) {
  const assignment = shift.assignment;
  if (assignment === null) return null;

  const job = assignment.job;
  const command = nextCommand(job);
  // Before the pickup the driver navigates to it; after, to the destination.
  const heading =
    job.status === 'IN_PROGRESS' || job.status === 'AT_DROPOFF'
      ? stopOf(job.stops, 'DROPOFF')
      : stopOf(job.stops, 'PICKUP');

  return (
    <View style={styles.screen} testID="driver-trip-screen">
      <View style={styles.card}>
        <Text style={styles.stage} testID="trip-stage">
          {STAGE[job.status] ?? job.status}
        </Text>
        <Text style={styles.label}>
          {job.status === 'IN_PROGRESS' || job.status === 'AT_DROPOFF'
            ? 'Destination'
            : 'Pickup'}
        </Text>
        <Text style={styles.value} testID="trip-heading">
          {describe(heading)}
        </Text>
      </View>

      {shift.error !== null && (
        <Text style={styles.error} testID="trip-error">
          {shift.error}
        </Text>
      )}

      {command !== null ? (
        <Pressable
          testID="trip-advance"
          style={({ pressed }) => [styles.button, pressed && styles.pressed]}
          disabled={shift.pending}
          onPress={() => void shift.advance()}
        >
          {shift.pending ? (
            <ActivityIndicator color={tokens.color.text} />
          ) : (
            <Text style={styles.buttonText}>{command.label}</Text>
          )}
        </Pressable>
      ) : (
        <Text style={styles.detail} testID="trip-no-command">
          Nothing to do here — waiting for the next job.
        </Text>
      )}
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
  card: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    marginBottom: tokens.space.md,
  },
  stage: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.lg,
    fontWeight: '600',
    marginBottom: tokens.space.md,
  },
  label: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  value: { color: tokens.color.text, fontSize: tokens.fontSize.md, marginTop: tokens.space.xs },
  detail: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md, textAlign: 'center' },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginBottom: tokens.space.md },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  pressed: { opacity: 0.8 },
});
