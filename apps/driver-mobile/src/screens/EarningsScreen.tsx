import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { formatMoney, tokens } from '@platform/ui';
import type { ApiClient, DriverEarnings } from '@platform/api-client';

/**
 * What the driver has made.
 *
 * A driver opens this at the end of a shift, and after a trip they believe was
 * underpaid. The second question is why the individual trips sit under the
 * totals: a single figure cannot answer it, and a driver who cannot check a
 * trip will ask support instead.
 */
export function EarningsScreen({ client, onBack }: { client: ApiClient; onBack(): void }) {
  const [earnings, setEarnings] = useState<DriverEarnings | null>(null);
  const [failed, setFailed] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setEarnings(await client.driverEarnings());
      setFailed(false);
    } catch {
      // Never fall back to zero. A driver shown PKR 0 because the books were
      // unreachable would believe they had worked a shift for nothing.
      setEarnings(null);
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="earnings-screen">
      <View style={styles.header}>
        <Text style={styles.title}>Earnings</Text>
        <Pressable testID="earnings-back" onPress={onBack}>
          <Text style={styles.back}>Done</Text>
        </Pressable>
      </View>

      {loading && earnings === null && !failed && (
        <ActivityIndicator testID="earnings-loading" color={tokens.color.textMuted} />
      )}

      {failed && (
        <View testID="earnings-error">
          <Text style={styles.error}>
            Your earnings could not be loaded. This does not affect what you have been paid.
          </Text>
          <Pressable testID="earnings-retry" onPress={() => void load()}>
            <Text style={styles.retry}>Try again</Text>
          </Pressable>
        </View>
      )}

      {earnings !== null && (
        <>
          <View style={styles.totals}>
            <Total
              testID="earnings-today"
              label="Today"
              amount={formatMoney(earnings.today.net)}
              trips={earnings.today.trips}
            />
            <Total
              testID="earnings-week"
              label="Last 7 days"
              amount={formatMoney(earnings.week.net)}
              trips={earnings.week.trips}
            />
          </View>

          {/* BD-05 is a flat 20% commission, so the figure above is not what
              the customer paid. Saying so once here is cheaper than answering
              it one driver at a time. */}
          <Text style={styles.note} testID="earnings-note">
            Amounts are what you keep, after the platform fee.
          </Text>

          {earnings.trips.length === 0 ? (
            <Text style={styles.empty} testID="earnings-empty">
              No trips in the last 7 days.
            </Text>
          ) : (
            earnings.trips.map((trip, index) => (
              <View
                key={`${trip.jobId}-${index}`}
                style={styles.trip}
                testID={`earnings-trip-${index}`}
              >
                <View style={styles.tripText}>
                  <Text style={styles.tripAmount}>{formatMoney(trip.amount)}</Text>
                  <Text style={styles.tripWhen}>{formatWhen(trip.at)}</Text>
                </View>
              </View>
            ))
          )}
        </>
      )}
    </ScrollView>
  );
}

function Total({
  testID,
  label,
  amount,
  trips,
}: {
  testID: string;
  label: string;
  amount: string;
  trips: number;
}) {
  return (
    <View style={styles.total} testID={testID}>
      <Text style={styles.totalLabel}>{label}</Text>
      <Text style={styles.totalAmount}>{amount}</Text>
      <Text style={styles.totalTrips}>
        {trips} {trips === 1 ? 'trip' : 'trips'}
      </Text>
    </View>
  );
}

/**
 * A time a driver can place without doing arithmetic.
 *
 * Deliberately not a locale-aware library: one more dependency for a line of
 * text, on an app that must stay small on a low-end phone.
 */
function formatWhen(at: Date): string {
  const hours = at.getHours().toString().padStart(2, '0');
  const minutes = at.getMinutes().toString().padStart(2, '0');
  const isToday = at.toDateString() === new Date().toDateString();
  if (isToday) return `${hours}:${minutes}`;
  return `${at.getDate()}/${at.getMonth() + 1} ${hours}:${minutes}`;
}

const styles = StyleSheet.create({
  screen: { padding: tokens.space.lg, backgroundColor: tokens.color.background, flexGrow: 1 },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: tokens.space.md,
  },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: '600' },
  back: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  totals: { flexDirection: 'row', gap: tokens.space.sm },
  total: {
    flex: 1,
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
  },
  totalLabel: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  totalAmount: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.lg,
    fontWeight: '600',
    marginTop: tokens.space.xs,
  },
  totalTrips: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  note: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.sm,
    marginBottom: tokens.space.md,
  },
  trip: {
    borderBottomColor: tokens.color.border,
    borderBottomWidth: 1,
    paddingVertical: tokens.space.sm,
  },
  tripText: { flexDirection: 'row', justifyContent: 'space-between' },
  tripAmount: { color: tokens.color.text, fontSize: tokens.fontSize.md },
  tripWhen: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  empty: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.md,
    marginBottom: tokens.space.sm,
  },
  retry: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
});
