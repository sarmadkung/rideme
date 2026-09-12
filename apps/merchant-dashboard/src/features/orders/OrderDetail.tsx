import { useState, type CSSProperties } from 'react';
import type { ItemIssueInput } from '@platform/api-client';
import type { MerchantOrder, MerchantOrderItem } from '@platform/types';
import { formatMoney, tokens } from '@platform/ui';
import { availableActions, statusLabel } from './actions';

export interface OrderDetailProps {
  order: MerchantOrder;
  working: boolean;
  error: string | null;
  onAccept(): void;
  onReject(reason: string): void;
  onStartPreparing(): void;
  onMarkReady(): void;
  onReportIssue(itemId: string, issue: ItemIssueInput): void;
}

/**
 * One order, with everything a picker needs and nothing they do not.
 *
 * The response carries no customer identity by design, so this shows what to
 * pick, by when, what was already reported against it, and — once it is ready
 * — which delivery it became.
 */
export function OrderDetail(props: OrderDetailProps) {
  const { order, working } = props;
  const actions = availableActions(order.status);
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState('');
  const [issueFor, setIssueFor] = useState<string | null>(null);

  return (
    <section style={styles.panel} aria-label="Order detail">
      <header style={styles.header}>
        <div>
          <h2 style={styles.heading}>#{order.id.slice(0, 8)}</h2>
          <p style={styles.muted}>{statusLabel(order.status)}</p>
        </div>
        <strong style={styles.total}>{formatMoney(order.items_total)}</strong>
      </header>

      {order.rejection_reason ? (
        <p style={styles.muted}>You rejected this: {order.rejection_reason}</p>
      ) : null}
      {order.job_id ? (
        <p style={styles.muted}>Delivery {order.job_id.slice(0, 8)} is on its way to collect.</p>
      ) : null}

      {props.error ? (
        <p role="alert" style={styles.error}>
          {props.error}
        </p>
      ) : null}

      <ul style={styles.items}>
        {(order.items ?? []).map((item) => (
          <li key={item.id} style={styles.item}>
            <div>
              <div>
                {item.quantity} × {item.name}
              </div>
              <div style={styles.muted}>
                {preferenceLabel(item)}
                {item.status === 'SUBSTITUTED' ? ' · substituted' : ''}
                {item.status === 'REMOVED' ? ' · removed' : ''}
              </div>
            </div>
            <div style={styles.itemRight}>
              <span>{formatMoney(item.line_total)}</span>
              {actions.includes('issue') ? (
                <button
                  style={styles.link}
                  type="button"
                  onClick={() => setIssueFor(issueFor === item.id ? null : item.id)}
                >
                  {issueFor === item.id ? 'Cancel' : 'Report issue'}
                </button>
              ) : null}
            </div>
            {issueFor === item.id ? (
              <IssueForm
                item={item}
                working={working}
                onSubmit={(issue) => {
                  props.onReportIssue(item.id, issue);
                  setIssueFor(null);
                }}
              />
            ) : null}
          </li>
        ))}
      </ul>

      {(order.issues ?? []).length > 0 ? (
        <section aria-label="Reported issues" style={styles.issues}>
          <h3 style={styles.subheading}>Reported</h3>
          {(order.issues ?? []).map((issue) => (
            <p key={issue.id} style={styles.muted}>
              {issue.reason} — {issueOutcome(issue.action, issue.resolution)}
              {issue.substitute_name ? `: ${issue.substitute_name}` : ''}
              {issue.substitute_price ? ` at ${formatMoney(issue.substitute_price)}` : ''}
            </p>
          ))}
        </section>
      ) : null}

      <footer style={styles.footer}>
        {actions.includes('accept') ? (
          <button style={styles.primary} type="button" disabled={working} onClick={props.onAccept}>
            Accept
          </button>
        ) : null}
        {actions.includes('preparing') ? (
          <button
            style={styles.primary}
            type="button"
            disabled={working}
            onClick={props.onStartPreparing}
          >
            Start preparing
          </button>
        ) : null}
        {actions.includes('ready') ? (
          <button
            style={styles.primary}
            type="button"
            disabled={working}
            onClick={props.onMarkReady}
          >
            Mark ready
          </button>
        ) : null}
        {actions.includes('reject') ? (
          <button
            style={styles.danger}
            type="button"
            disabled={working}
            onClick={() => setRejecting(true)}
          >
            Reject
          </button>
        ) : null}
      </footer>

      {rejecting ? (
        <form
          style={styles.rejectForm}
          onSubmit={(event) => {
            event.preventDefault();
            props.onReject(reason.trim());
            setRejecting(false);
            setReason('');
          }}
        >
          <label style={styles.label} htmlFor="reject-reason">
            Why are you rejecting this?
          </label>
          <input
            id="reject-reason"
            style={styles.input}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
          {/* The server requires a reason. Enforcing it here too means the
              shop is told before the round trip, not after it. */}
          <button style={styles.danger} type="submit" disabled={working || reason.trim() === ''}>
            Reject this order
          </button>
        </form>
      ) : null}
    </section>
  );
}

/**
 * The issue form.
 *
 * It offers what the shop can propose. What happens is the customer's standing
 * preference applied server-side, which is why the form says "propose" and the
 * reported list above says what actually became of it.
 */
function IssueForm({
  item,
  working,
  onSubmit,
}: {
  item: MerchantOrderItem;
  working: boolean;
  onSubmit(issue: ItemIssueInput): void;
}) {
  const [reason, setReason] = useState('');
  const [substitute, setSubstitute] = useState('');
  const [price, setPrice] = useState('');

  const substituting = substitute.trim() !== '';
  const priceMinor = Math.round(Number(price) * 100);
  const priceUsable = Number.isFinite(priceMinor) && price.trim() !== '' && priceMinor > 0;

  return (
    <form
      style={styles.issueForm}
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(
          substituting
            ? {
                reason: reason.trim(),
                action: 'SUBSTITUTE',
                substituteName: substitute.trim(),
                substitutePriceMinor: priceMinor,
              }
            : { reason: reason.trim(), action: 'REMOVE' },
        );
      }}
    >
      <label style={styles.label} htmlFor={`reason-${item.id}`}>
        What is wrong?
      </label>
      <input
        id={`reason-${item.id}`}
        style={styles.input}
        value={reason}
        onChange={(event) => setReason(event.target.value)}
        placeholder="Out of stock"
      />
      <label style={styles.label} htmlFor={`substitute-${item.id}`}>
        Offer instead (leave empty to remove the line)
      </label>
      <input
        id={`substitute-${item.id}`}
        style={styles.input}
        value={substitute}
        onChange={(event) => setSubstitute(event.target.value)}
      />
      {substituting ? (
        <>
          <label style={styles.label} htmlFor={`price-${item.id}`}>
            Its price, per unit
          </label>
          <input
            id={`price-${item.id}`}
            style={styles.input}
            inputMode="decimal"
            value={price}
            onChange={(event) => setPrice(event.target.value)}
          />
        </>
      ) : null}
      {/* A substitute with no price is one nobody can be charged for, so the
          server refuses it; the button refuses it first. */}
      <button
        style={styles.primary}
        type="submit"
        disabled={working || reason.trim() === '' || (substituting && !priceUsable)}
      >
        {substituting ? 'Offer this substitute' : 'Remove this line'}
      </button>
    </form>
  );
}

function preferenceLabel(item: MerchantOrderItem): string {
  switch (item.substitution_preference) {
    case 'ALLOW':
      return 'Substitutes fine';
    case 'DO_NOT_ALLOW':
      return 'No substitutes';
    case 'ASK_ME':
      return 'Ask before substituting';
    default:
      return item.substitution_preference;
  }
}

function issueOutcome(action: string, resolution: string): string {
  if (resolution === 'PENDING') return 'waiting on the customer';
  if (resolution === 'CUSTOMER_DECLINED') return 'declined, line removed';
  if (resolution === 'CUSTOMER_ACCEPTED') return 'accepted by the customer';
  if (action === 'REMOVE') return 'removed';
  return 'applied';
}

const styles: Record<string, CSSProperties> = {
  panel: {
    display: 'flex',
    flexDirection: 'column',
    gap: tokens.space.sm,
    padding: tokens.space.md,
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.md,
  },
  header: { display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' },
  heading: { fontSize: tokens.fontSize.lg, margin: 0 },
  subheading: { fontSize: tokens.fontSize.md, margin: 0 },
  total: { fontSize: tokens.fontSize.lg },
  muted: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, margin: 0 },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, margin: 0 },
  items: { listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: tokens.space.sm },
  item: {
    display: 'grid',
    gridTemplateColumns: '1fr auto',
    gap: tokens.space.xs,
    paddingBottom: tokens.space.sm,
    borderBottom: `1px solid ${tokens.color.border}`,
    fontSize: tokens.fontSize.md,
  },
  itemRight: { display: 'flex', flexDirection: 'column', alignItems: 'flex-end' },
  issues: { display: 'grid', gap: tokens.space.xs },
  footer: { display: 'flex', gap: tokens.space.sm, flexWrap: 'wrap' },
  rejectForm: { display: 'grid', gap: tokens.space.xs },
  issueForm: { gridColumn: '1 / -1', display: 'grid', gap: tokens.space.xs },
  label: { fontSize: tokens.fontSize.sm, color: tokens.color.textMuted },
  input: {
    padding: tokens.space.sm,
    fontSize: tokens.fontSize.md,
    background: tokens.color.background,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
  },
  primary: {
    padding: tokens.space.sm,
    background: tokens.color.accent,
    color: tokens.color.text,
    border: 'none',
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  danger: {
    padding: tokens.space.sm,
    background: tokens.color.danger,
    color: tokens.color.text,
    border: 'none',
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  link: {
    background: 'none',
    border: 'none',
    color: tokens.color.accent,
    fontSize: tokens.fontSize.sm,
    cursor: 'pointer',
    padding: 0,
  },
};
