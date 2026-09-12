import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';
import { useState } from 'react';
import { formatMoney, tokens } from '@platform/ui';
import type { ApiClient, StopInput } from '@platform/api-client';
import { PlacePicker } from '../components/PlacePicker';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

/**
 * Where it goes (document 71's Address step).
 *
 * The order cannot be picked without this: `MarkReady` refuses an order with
 * no destination, because a delivery job needs both ends of a route before a
 * driver can take it. So it is asked here, before the shop is committed,
 * rather than chased afterwards by support.
 */
export function GroceryCheckoutScreen({
  grocery,
  client,
}: {
  grocery: GroceryState & GroceryActions;
  client: ApiClient;
}) {
  const [stop, setStop] = useState<StopInput | null>(null);
  const [name, setName] = useState<string | null>(null);
  const [address, setAddress] = useState('');
  const [notes, setNotes] = useState('');

  const cart = grocery.order;
  const ready = stop !== null && address.trim() !== '' && !grocery.pending;

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="checkout-screen">
      <Pressable testID="checkout-back" onPress={grocery.backToShopping}>
        <Text style={styles.back}>‹ Back to the shop</Text>
      </Pressable>
      <Text style={styles.title}>Where should it go?</Text>

      <PlacePicker
        label="Delivery address"
        testID="checkout-address"
        selected={stop}
        selectedName={name}
        disabled={grocery.pending}
        client={client}
        placeholder="Search for your address"
        onSelect={(chosen, chosenName, chosenAddress) => {
          setStop(chosen);
          setName(chosenName);
          // The geocoder's line where there is one, the place's name where
          // there is not. Either way it is editable below: a rider needs a
          // flat number, and no geocoder knows it.
          setAddress(chosen === null ? '' : (chosenAddress ?? chosenName ?? ''));
        }}
      />

      {stop !== null && (
        <>
          <Text style={styles.label}>Address as the rider will read it</Text>
          <TextInput
            testID="checkout-address-text"
            style={styles.input}
            value={address}
            editable={!grocery.pending}
            multiline
            placeholder="House 12, Street 4, Gulberg"
            placeholderTextColor={tokens.color.textMuted}
            onChangeText={setAddress}
          />

          <Text style={styles.label}>Anything the rider should know</Text>
          <TextInput
            testID="checkout-notes"
            style={styles.input}
            value={notes}
            editable={!grocery.pending}
            placeholder="Second gate, ring the bell"
            placeholderTextColor={tokens.color.textMuted}
            onChangeText={setNotes}
          />
        </>
      )}

      {cart && (
        <View style={styles.summary} testID="checkout-summary">
          {cart.items.map((line) => (
            <View key={line.id} style={styles.row}>
              <Text style={styles.rowLabel} numberOfLines={1}>
                {line.quantity} × {line.name}
              </Text>
              <Text style={styles.rowValue}>{formatMoney(line.line_total)}</Text>
            </View>
          ))}
          <View style={[styles.row, styles.totalRow]}>
            <Text style={styles.totalLabel}>Items</Text>
            <Text style={styles.totalLabel} testID="checkout-total">
              {formatMoney(cart.items_total)}
            </Text>
          </View>
          {/* Delivery is quoted when the order becomes a job, which happens
              when the shop marks it ready. Showing a made-up number here would
              be the client computing money. */}
          <Text style={styles.note}>Delivery is charged separately once a rider is assigned.</Text>
        </View>
      )}

      {grocery.error !== null && (
        <Text style={styles.error} testID="checkout-error">
          {grocery.error}
        </Text>
      )}

      <Pressable
        testID="checkout-place"
        disabled={!ready}
        style={[styles.button, !ready && styles.buttonDisabled]}
        onPress={() => {
          if (stop === null) return;
          void grocery.place({
            address: address.trim(),
            latitude: stop.latitude,
            longitude: stop.longitude,
            ...(notes.trim() ? { notes: notes.trim() } : {}),
          });
        }}
      >
        <Text style={styles.buttonText}>{grocery.pending ? 'Placing…' : 'Place order'}</Text>
      </Pressable>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { backgroundColor: tokens.color.background, padding: tokens.space.lg, flexGrow: 1 },
  back: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  title: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.xl,
    fontWeight: '600',
    marginBottom: tokens.space.lg,
  },
  label: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.xs,
  },
  input: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    color: tokens.color.text,
    fontSize: tokens.fontSize.md,
    padding: tokens.space.sm,
    marginBottom: tokens.space.md,
  },
  summary: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    paddingTop: tokens.space.md,
    marginTop: tokens.space.sm,
  },
  row: { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: tokens.space.xs },
  rowLabel: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, flexShrink: 1 },
  rowValue: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  totalRow: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    marginTop: tokens.space.xs,
    paddingTop: tokens.space.sm,
  },
  totalLabel: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  note: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, marginTop: tokens.space.sm },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.lg,
  },
  buttonDisabled: { opacity: 0.5 },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginTop: tokens.space.md },
});
