import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import type { ApiClient } from '@platform/api-client';
import type { MerchantOrder, MerchantQueue } from '@platform/types';
import { MERCHANT_QUEUES } from '@platform/types';
import { formatMoney, tokens } from '@platform/ui';
import { OrderDetail } from '../features/orders/OrderDetail';
import { QUEUE_LABELS, minutesLeft, statusLabel } from '../features/orders/actions';
import { useOrder } from '../features/orders/useOrder';
import { useQueue } from '../features/orders/useQueue';

/**
 * The shop's working screen: document 72's five queues beside one order.
 *
 * It opens on New because that is the queue with a clock on it — BD-12 cancels
 * an unanswered order, so the only queue where doing nothing is a decision is
 * the one a shop should be looking at.
 */
export function OrdersScreen({ client }: { client: ApiClient }) {
  const [queue, setQueue] = useState<MerchantQueue>('new');
  const [selected, setSelected] = useState<string | null>(null);
  const list = useQueue(client, queue);
  const order = useOrder(client, selected, list.refresh);

  // A minute hand for the accept deadline. Without it the countdown is only
  // as fresh as the last poll, and "3m left" would sit there after it expired.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);

  // The panel follows the order, not the queue. Accepting one moves it out of
  // New, and a screen that blanked the moment the shop acted would hide the
  // result of the tap and the "Start preparing" button that follows it.
  const select = useCallback((id: string) => setSelected(id), []);

  return (
    <main style={styles.page}>
      <nav style={styles.tabs} aria-label="Order queues">
        {MERCHANT_QUEUES.map((name) => (
          <button
            key={name}
            type="button"
            style={name === queue ? styles.tabActive : styles.tab}
            aria-current={name === queue}
            onClick={() => {
              setQueue(name);
              setSelected(null);
            }}
          >
            {QUEUE_LABELS[name]}
          </button>
        ))}
      </nav>

      <div style={styles.columns}>
        <section style={styles.list} aria-label={`${QUEUE_LABELS[queue]} orders`}>
          {list.error ? (
            <p role="alert" style={styles.error}>
              {list.error}
            </p>
          ) : null}
          {!list.loading && list.orders.length === 0 && !list.error ? (
            <p style={styles.muted}>Nothing here.</p>
          ) : null}
          {list.orders.map((item) => (
            <QueueRow
              key={item.id}
              order={item}
              now={now}
              selected={item.id === selected}
              onSelect={select}
            />
          ))}
          {list.hasMore ? (
            <button style={styles.more} type="button" onClick={() => void list.loadMore()}>
              Show older
            </button>
          ) : null}
        </section>

        <div>
          {order.order ? (
            <OrderDetail
              order={order.order}
              working={order.working}
              error={order.error}
              onAccept={() => void order.accept()}
              onReject={(reason) => void order.reject(reason)}
              onStartPreparing={() => void order.startPreparing()}
              onMarkReady={() => void order.markReady()}
              onReportIssue={(itemId, issue) => void order.reportIssue(itemId, issue)}
            />
          ) : (
            <p style={styles.muted}>Choose an order.</p>
          )}
        </div>
      </div>
    </main>
  );
}

function QueueRow({
  order,
  now,
  selected,
  onSelect,
}: {
  order: MerchantOrder;
  now: number;
  selected: boolean;
  onSelect(id: string): void;
}) {
  const left = minutesLeft(order, now);
  return (
    <button
      type="button"
      style={selected ? styles.rowSelected : styles.row}
      onClick={() => onSelect(order.id)}
    >
      <span>#{order.id.slice(0, 8)}</span>
      <span style={styles.muted}>{statusLabel(order.status)}</span>
      <span>{formatMoney(order.items_total)}</span>
      {left === null ? null : (
        <span style={left <= 2 ? styles.urgent : styles.muted}>{left}m to answer</span>
      )}
    </button>
  );
}

const styles: Record<string, CSSProperties> = {
  page: {
    minHeight: '100vh',
    background: tokens.color.background,
    color: tokens.color.text,
    fontFamily: 'system-ui, sans-serif',
    padding: tokens.space.md,
    display: 'grid',
    gap: tokens.space.md,
    alignContent: 'start',
  },
  tabs: { display: 'flex', gap: tokens.space.sm, flexWrap: 'wrap' },
  tab: {
    padding: `${tokens.space.xs}px ${tokens.space.md}px`,
    background: tokens.color.surface,
    color: tokens.color.textMuted,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  tabActive: {
    padding: `${tokens.space.xs}px ${tokens.space.md}px`,
    background: tokens.color.accent,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.accent}`,
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  columns: { display: 'grid', gridTemplateColumns: 'minmax(280px, 1fr) 2fr', gap: tokens.space.md },
  list: { display: 'grid', gap: tokens.space.sm, alignContent: 'start' },
  row: {
    display: 'grid',
    gridTemplateColumns: '1fr 1fr auto',
    gap: tokens.space.xs,
    textAlign: 'left',
    padding: tokens.space.sm,
    background: tokens.color.surface,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.md,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  rowSelected: {
    display: 'grid',
    gridTemplateColumns: '1fr 1fr auto',
    gap: tokens.space.xs,
    textAlign: 'left',
    padding: tokens.space.sm,
    background: tokens.color.surface,
    color: tokens.color.text,
    border: `1px solid ${tokens.color.accent}`,
    borderRadius: tokens.radius.md,
    cursor: 'pointer',
    fontSize: tokens.fontSize.md,
  },
  more: {
    padding: tokens.space.sm,
    background: 'none',
    color: tokens.color.textMuted,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
    cursor: 'pointer',
  },
  muted: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, margin: 0 },
  urgent: { color: tokens.color.warning, fontSize: tokens.fontSize.sm },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, margin: 0 },
};
