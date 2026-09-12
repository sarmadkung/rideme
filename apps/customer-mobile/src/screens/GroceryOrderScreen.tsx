import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { formatMoney, tokens } from '@platform/ui';
import type { GroceryOrder, OrderIssue } from '@platform/types';
import {
  isCancellable,
  isFinished,
  pendingIssue,
  type GroceryActions,
  type GroceryState,
} from '../features/grocery/useGrocery';

/** The lifecycle in the customer's words (document 70). */
const STATUS: Record<string, string> = {
  PLACED: 'Sent to the shop',
  PAYMENT_PENDING: 'Waiting for payment',
  CONFIRMED: 'The shop accepted it',
  PREPARING: 'Being picked',
  READY_FOR_PICKUP: 'Waiting for a rider',
  PICKED_UP: 'A rider has it',
  DELIVERING: 'On its way to you',
  DELIVERED: 'Delivered',
  CANCELLED: 'Cancelled',
  FAILED: 'Something went wrong',
};

export function statusLabel(status: string): string {
  return STATUS[status] ?? status;
}

export function GroceryOrderScreen({ grocery }: { grocery: GroceryState & GroceryActions }) {
  const order = grocery.order;
  if (!order) return null;

  const question = pendingIssue(order);

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="grocery-order-screen">
      <Text style={styles.title} testID="order-status">
        {statusLabel(order.status)}
      </Text>
      {order.delivery ? (
        <Text style={styles.meta} numberOfLines={2}>
          To {order.delivery.address}
        </Text>
      ) : null}

      {/* The question comes first, above everything else on the screen. A
          picker is standing at a shelf waiting for this answer, and it is the
          one thing here that only the customer can do. */}
      {question !== null && (
        <SubstitutionQuestion
          issue={question}
          order={order}
          pending={grocery.pending}
          onDecide={(accept) => void grocery.decide(question.id, accept)}
        />
      )}

      {grocery.error !== null && (
        <Text style={styles.error} testID="order-error">
          {grocery.error}
        </Text>
      )}

      <View style={styles.panel}>
        {order.items.map((line) => (
          <View key={line.id} style={styles.row} testID={`order-line-${line.id}`}>
            <Text
              style={[styles.rowLabel, line.status === 'REMOVED' && styles.removed]}
              numberOfLines={1}
            >
              {line.quantity} × {line.name}
              {line.status === 'REMOVED' ? ' · not available' : ''}
              {line.status === 'SUBSTITUTED' ? ' · substituted' : ''}
            </Text>
            <Text style={styles.rowValue}>{formatMoney(line.line_total)}</Text>
          </View>
        ))}
        <View style={[styles.row, styles.totalRow]}>
          <Text style={styles.totalLabel}>Items</Text>
          <Text style={styles.totalLabel} testID="order-total">
            {formatMoney(order.items_total)}
          </Text>
        </View>
      </View>

      {isCancellable(order) && (
        <Pressable
          testID="order-cancel"
          disabled={grocery.pending}
          style={styles.cancel}
          onPress={() => void grocery.cancel()}
        >
          {/* No reason is asked for. A customer who changed their mind owes
              nobody an explanation, and a required field collects "asdf". */}
          <Text style={styles.cancelText}>Cancel this order</Text>
        </Pressable>
      )}

      {isFinished(order) && (
        <Pressable testID="order-done" style={styles.button} onPress={grocery.reset}>
          <Text style={styles.buttonText}>Order again</Text>
        </Pressable>
      )}
    </ScrollView>
  );
}

function SubstitutionQuestion({
  issue,
  order,
  pending,
  onDecide,
}: {
  issue: OrderIssue;
  order: GroceryOrder;
  pending: boolean;
  onDecide(accept: boolean): void;
}) {
  const line = order.items.find((item) => item.id === issue.order_item_id);

  return (
    <View style={styles.question} testID="order-question">
      <Text style={styles.questionTitle}>The shop could not find {line?.name ?? 'an item'}</Text>
      <Text style={styles.questionBody}>{issue.reason}</Text>
      {issue.substitute_name ? (
        <Text style={styles.questionBody} testID="order-question-substitute">
          They can send {issue.substitute_name}
          {issue.substitute_price ? ` at ${formatMoney(issue.substitute_price)}` : ''}
          {/* BD-11 sends the difference in both directions: a cheaper
              substitute is a refund, and saying so is the difference between
              a fair swap and a surprise on the bill. */}
          {issue.price_difference
            ? ` — ${formatMoney(issue.price_difference)} against your total`
            : ''}
          .
        </Text>
      ) : null}

      <View style={styles.questionButtons}>
        <Pressable
          testID="order-question-accept"
          disabled={pending}
          style={styles.button}
          onPress={() => onDecide(true)}
        >
          <Text style={styles.buttonText}>Send it</Text>
        </Pressable>
        <Pressable
          testID="order-question-decline"
          disabled={pending}
          style={styles.secondary}
          onPress={() => onDecide(false)}
        >
          {/* Declining drops the line rather than restoring it: the shelf is
              empty, which is why they were asked. */}
          <Text style={styles.secondaryText}>Leave it out</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { backgroundColor: tokens.color.background, padding: tokens.space.lg, flexGrow: 1 },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: '600' },
  meta: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.xs,
    marginBottom: tokens.space.lg,
  },
  question: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.warning,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    marginBottom: tokens.space.lg,
  },
  questionTitle: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  questionBody: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.xs,
  },
  questionButtons: { flexDirection: 'row', gap: tokens.space.sm, marginTop: tokens.space.md },
  panel: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
  },
  row: { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: tokens.space.xs },
  rowLabel: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, flexShrink: 1 },
  removed: { textDecorationLine: 'line-through' },
  rowValue: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  totalRow: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    marginTop: tokens.space.xs,
    paddingTop: tokens.space.sm,
  },
  totalLabel: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.md,
    flexGrow: 1,
  },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  secondary: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.md,
    flexGrow: 1,
  },
  secondaryText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.md },
  cancel: { padding: tokens.space.md, alignItems: 'center', marginTop: tokens.space.md },
  cancelText: { color: tokens.color.danger, fontSize: tokens.fontSize.sm },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
});
