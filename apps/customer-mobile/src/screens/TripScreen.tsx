import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { formatMoney, tokens } from '@platform/ui';
import type { Job } from '@platform/types';
import {
  isCancellable,
  isFinished,
  type BookingActions,
  type BookingState,
} from '../features/booking/useBooking';

/**
 * What the customer sees while their ride happens (document 036).
 *
 * Every status the server can report has a line here. A status with no case
 * would leave a customer staring at a raw enum, so the fallback shows the
 * status rather than nothing.
 */
const STATUS_TEXT: Record<string, { title: string; detail: string }> = {
  DRAFT: { title: 'Getting ready', detail: 'Setting up your ride.' },
  QUOTED: { title: 'Getting ready', detail: 'Setting up your ride.' },
  REQUESTED: { title: 'Requesting', detail: 'Sending your request.' },
  SEARCHING: { title: 'Finding a driver', detail: 'Looking for drivers nearby.' },
  ASSIGNED: { title: 'Driver found', detail: 'Waiting for them to confirm.' },
  ACCEPTED: { title: 'Driver confirmed', detail: 'They are on their way.' },
  ARRIVING: { title: 'Driver arriving', detail: 'Your driver is close.' },
  AT_PICKUP: { title: 'Driver has arrived', detail: 'Your driver is waiting at the pickup point.' },
  IN_PROGRESS: { title: 'On the way', detail: 'Enjoy your ride.' },
  AT_DROPOFF: { title: 'Arriving', detail: 'You have reached your destination.' },
  COMPLETED: { title: 'Trip complete', detail: 'Thanks for riding with us.' },
  CANCELLED: { title: 'Ride cancelled', detail: 'This ride was cancelled.' },
  // BD-04's outcome. It is stated plainly, and the customer is told they were
  // not charged, because "expired" on its own reads like something went wrong
  // with their payment.
  EXPIRED: {
    title: 'No drivers available',
    detail: 'We could not find a driver nearby. You have not been charged.',
  },
  FAILED: { title: 'Ride failed', detail: 'Something went wrong with this ride.' },
  DISPUTED: { title: 'Under review', detail: 'This ride is being reviewed.' },
};

function statusTextFor(job: Job) {
  return STATUS_TEXT[job.status] ?? { title: job.status, detail: '' };
}

export function TripScreen({ booking }: { booking: BookingState & BookingActions }) {
  const job = booking.job;
  if (job === null) return null;

  const status = statusTextFor(job);
  const finished = isFinished(job);
  const cancellation = booking.cancellation;

  return (
    <View style={styles.screen} testID="trip-screen">
      <View style={styles.card}>
        <Text style={styles.title} testID="trip-status">
          {status.title}
        </Text>
        {status.detail !== '' && <Text style={styles.detail}>{status.detail}</Text>}

        {job.assigned_driver_id !== undefined && !finished && (
          <Text style={styles.meta} testID="trip-driver">
            Driver assigned
          </Text>
        )}
      </View>

      {/* What the cancellation actually cost. BD-01 charges after two minutes
          from driver acceptance, and a customer must be told the amount here
          rather than discovering it on a statement. */}
      {cancellation !== null && (
        <View style={styles.card} testID="cancellation-summary">
          <Text style={styles.detail}>
            {cancellation.fee.amount_minor === 0
              ? 'You were not charged for this cancellation.'
              : `Cancellation fee: ${formatMoney(cancellation.fee)}`}
          </Text>
        </View>
      )}

      {booking.error !== null && (
        <Text style={styles.error} testID="trip-error">
          {booking.error}
        </Text>
      )}

      {isCancellable(job) && (
        <Pressable
          testID="cancel-ride"
          style={({ pressed }) => [styles.cancelButton, pressed && styles.pressed]}
          disabled={booking.pending}
          onPress={() => void booking.cancel('customer cancelled')}
        >
          {booking.pending ? (
            <ActivityIndicator color={tokens.color.danger} />
          ) : (
            <Text style={styles.cancelText}>Cancel ride</Text>
          )}
        </Pressable>
      )}

      {finished && (
        <Pressable
          testID="book-another"
          style={({ pressed }) => [styles.button, pressed && styles.pressed]}
          onPress={booking.reset}
        >
          <Text style={styles.buttonText}>Book another ride</Text>
        </Pressable>
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
  title: { color: tokens.color.text, fontSize: tokens.fontSize.lg, fontWeight: '600' },
  detail: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md, marginTop: tokens.space.xs },
  meta: { color: tokens.color.success, fontSize: tokens.fontSize.sm, marginTop: tokens.space.sm },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginBottom: tokens.space.md },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  cancelButton: {
    borderColor: tokens.color.danger,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  cancelText: { color: tokens.color.danger, fontSize: tokens.fontSize.md, fontWeight: '600' },
  pressed: { opacity: 0.8 },
});
