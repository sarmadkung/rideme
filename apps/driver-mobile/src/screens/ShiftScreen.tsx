import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { isOnline, type ShiftActions, type ShiftState } from '../features/shift/useShift';

/**
 * The idle screen: on or off duty, and why the driver might not be able to
 * work.
 *
 * A driver who cannot go online must be told which of the several possible
 * reasons applies. "Go online" that silently does nothing is the worst
 * version of this screen.
 */
export function ShiftScreen({
  shift,
  position,
}: {
  shift: ShiftState & ShiftActions;
  position: { latitude: number; longitude: number } | null;
}) {
  const driver = shift.driver;
  const online = isOnline(driver);
  const approved = driver?.verification_status === 'APPROVED';
  const hasVehicle = Boolean(driver?.active_vehicle_id);
  const blocker = !approved
    ? 'Your account is still being verified. You can go online once it is approved.'
    : !hasVehicle
      ? 'Select an active vehicle before going online.'
      : position === null
        ? 'Waiting for your location.'
        : null;

  return (
    <View style={styles.screen} testID="shift-screen">
      <View style={styles.card}>
        <Text style={styles.status} testID="shift-status">
          {online ? "You're online" : "You're offline"}
        </Text>
        <Text style={styles.detail}>
          {online ? 'Waiting for a job nearby.' : 'Go online to start receiving jobs.'}
        </Text>
      </View>

      {blocker !== null && !online && (
        <Text style={styles.blocker} testID="shift-blocker">
          {blocker}
        </Text>
      )}

      {shift.error !== null && (
        <Text style={styles.error} testID="shift-error">
          {shift.error}
        </Text>
      )}

      <Pressable
        testID="shift-toggle"
        style={({ pressed }) => [
          online ? styles.offlineButton : styles.onlineButton,
          pressed && styles.pressed,
        ]}
        disabled={shift.pending || (!online && blocker !== null)}
        onPress={() => {
          if (online) void shift.goOffline();
          else if (position !== null) void shift.goOnline(position);
        }}
      >
        {shift.pending ? (
          <ActivityIndicator color={tokens.color.text} />
        ) : (
          <Text style={styles.buttonText}>{online ? 'Go offline' : 'Go online'}</Text>
        )}
      </Pressable>
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
  status: { color: tokens.color.text, fontSize: tokens.fontSize.lg, fontWeight: '600' },
  detail: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md, marginTop: tokens.space.xs },
  blocker: {
    color: tokens.color.warning,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginBottom: tokens.space.md },
  onlineButton: {
    backgroundColor: tokens.color.success,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  offlineButton: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
  },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  pressed: { opacity: 0.8 },
});
