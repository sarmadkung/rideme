import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { isOnline, type ShiftActions, type ShiftState } from '../features/shift/useShift';
import type { LocationActions, LocationState } from '../features/location/useLocation';

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
  location,
  onShowEarnings,
}: {
  shift: ShiftState & ShiftActions;
  location: LocationState & LocationActions;
  onShowEarnings(): void;
}) {
  const driver = shift.driver;
  const online = isOnline(driver);
  const approved = driver?.verification_status === 'APPROVED';
  const hasVehicle = Boolean(driver?.active_vehicle_id);
  const position = location.position;
  // The location message is the hook's, not this screen's: refused, switched
  // off and simply not ready yet need different actions from the driver, and
  // one sentence covering all three sends most of them to the wrong setting.
  const blocker = !approved
    ? 'Your account is still being verified. You can go online once it is approved.'
    : !hasVehicle
      ? 'Select an active vehicle before going online.'
      : position === null
        ? location.message
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

      {!online && position === null && location.stage !== 'starting' && (
        <Pressable testID="location-retry" onPress={location.retry}>
          <Text style={styles.retry}>Check again</Text>
        </Pressable>
      )}

      <Pressable testID="show-earnings" onPress={onShowEarnings}>
        <Text style={styles.earningsLink}>See earnings</Text>
      </Pressable>

      {/* BD-09's refusal, shown ahead of the generic error because it is the
          only one the driver can act on themselves — and with the amount,
          because "you cannot go online" without a number is something a driver
          reads as the app being broken. */}
      {shift.blockedByCash ? (
        <View style={styles.blocked} testID="shift-blocked">
          <Text style={styles.blockedTitle}>Hand in platform cash to go online</Text>
          <Text style={styles.blockedBody}>
            You are holding {shift.blockedByCash.owed}, which is at your limit of{' '}
            {shift.blockedByCash.cap}. Hand in {shift.blockedByCash.clearing} at any office to start
            accepting trips again.
          </Text>
        </View>
      ) : (
        shift.error !== null && (
          <Text style={styles.error} testID="shift-error">
            {shift.error}
          </Text>
        )
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
          <ActivityIndicator color={online ? tokens.color.text : tokens.color.onAccent} />
        ) : (
          <Text style={[styles.buttonText, !online && styles.onFilled]}>
            {online ? 'Go offline' : 'Go online'}
          </Text>
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
  detail: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.xs,
  },
  blocker: {
    color: tokens.color.warning,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
  blocked: {
    backgroundColor: tokens.color.surface,
    borderRadius: tokens.radius.md,
    borderLeftWidth: 3,
    borderLeftColor: tokens.color.warning,
    padding: tokens.space.md,
    marginBottom: tokens.space.md,
    gap: tokens.space.xs,
  },
  blockedTitle: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.md,
    fontWeight: '600',
  },
  blockedBody: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
  },
  earningsLink: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.md,
    fontWeight: '600',
    marginBottom: tokens.space.md,
  },
  retry: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.sm,
    fontWeight: '600',
    marginBottom: tokens.space.md,
  },
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
  // The offline button is an outline on the dark screen, the online one a
  // green fill. One label serves both, so only the filled state takes the
  // dark ink — applying it unconditionally would hide 'Go offline'.
  onFilled: { color: tokens.color.onAccent },
  pressed: { opacity: 0.8 },
});
