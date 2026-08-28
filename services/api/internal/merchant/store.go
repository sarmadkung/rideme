package merchant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// --- configuration -----------------------------------------------------------

// Config loads a merchant's operational settings.
func (s *Store) Config(ctx context.Context, merchantID string) (Config, error) {
	var c Config
	var acceptSeconds, prepSeconds *int
	err := s.pool.QueryRow(ctx,
		`SELECT merchant_id::text, accept_timeout_seconds, default_prep_seconds, auto_accept
		   FROM merchant_config WHERE merchant_id = $1`, merchantID).
		Scan(&c.MerchantID, &acceptSeconds, &prepSeconds, &c.AutoAccept)
	if errors.Is(err, pgx.ErrNoRows) {
		return Config{MerchantID: merchantID}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("load merchant config: %w", err)
	}
	if acceptSeconds != nil {
		d := time.Duration(*acceptSeconds) * time.Second
		c.AcceptTimeout = &d
	}
	if prepSeconds != nil {
		d := time.Duration(*prepSeconds) * time.Second
		c.DefaultPrepTime = &d
	}
	return c, nil
}

// SetConfig stores a merchant's settings.
func (s *Store) SetConfig(ctx context.Context, c Config) error {
	var acceptSeconds, prepSeconds any
	if c.AcceptTimeout != nil {
		acceptSeconds = int(c.AcceptTimeout.Seconds())
	}
	if c.DefaultPrepTime != nil {
		prepSeconds = int(c.DefaultPrepTime.Seconds())
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO merchant_config (merchant_id, accept_timeout_seconds, default_prep_seconds, auto_accept)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (merchant_id) DO UPDATE SET
		   accept_timeout_seconds = EXCLUDED.accept_timeout_seconds,
		   default_prep_seconds = EXCLUDED.default_prep_seconds,
		   auto_accept = EXCLUDED.auto_accept, updated_at = now()`,
		c.MerchantID, acceptSeconds, prepSeconds, c.AutoAccept)
	if err != nil {
		return fmt.Errorf("save merchant config: %w", err)
	}
	return nil
}

// --- cart and orders ---------------------------------------------------------

const orderColumns = `id::text, merchant_id::text, COALESCE(store_id::text, ''),
	customer_user_id::text, status, COALESCE(job_id::text, ''), currency, items_total_minor,
	accepted_at, preparation_started_at, ready_at, expected_ready_at, accept_deadline,
	COALESCE(rejection_reason, ''), created_at, updated_at`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var currency money.Currency
	var totalMinor int64
	err := row.Scan(&o.ID, &o.MerchantID, &o.StoreID, &o.CustomerUserID, &o.Status, &o.JobID,
		&currency, &totalMinor, &o.AcceptedAt, &o.PreparationStart, &o.ReadyAt,
		&o.ExpectedReadyAt, &o.AcceptDeadline, &o.RejectionReason, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return Order{}, err
	}
	o.ItemsTotal, err = money.New(totalMinor, currency)
	return o, err
}

// OpenCart returns the customer's cart for a store, creating one if needed.
func (s *Store) OpenCart(ctx context.Context, merchantID, storeID, customerID string) (Order, error) {
	order, err := scanOrder(s.pool.QueryRow(ctx,
		`INSERT INTO orders (merchant_id, store_id, customer_user_id, status)
		 VALUES ($1, $2, $3, 'CART')
		 ON CONFLICT (customer_user_id, store_id) WHERE status = 'CART'
		 DO UPDATE SET updated_at = now()
		 RETURNING `+orderColumns,
		merchantID, storeID, customerID))
	if err != nil {
		return Order{}, fmt.Errorf("open cart: %w", err)
	}
	return order, nil
}

// AddItem adds a line to a cart, snapshotting the product's price.
//
// The snapshot is the point: document 68 requires that later catalogue changes
// never alter historical orders, and referencing the live price would rewrite
// every past receipt the next time a merchant edited one.
func (s *Store) AddItem(ctx context.Context, orderID, productID, variantID string, quantity int, preference SubstitutionPreference) (Item, error) {
	if quantity <= 0 {
		return Item{}, errors.New("merchant: quantity must be positive")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var name string
	var priceMinor int64
	var currency money.Currency
	var status string
	err = tx.QueryRow(ctx,
		`SELECT p.name, p.price_minor + COALESCE(v.price_delta_minor, 0), p.currency, p.status
		   FROM products p
		   LEFT JOIN product_variants v ON v.id = $2 AND v.product_id = p.id
		  WHERE p.id = $1`, productID, nullableUUID(variantID)).
		Scan(&name, &priceMinor, &currency, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("load product: %w", err)
	}
	if status != "ACTIVE" {
		return Item{}, fmt.Errorf("%w: the product is %s", ErrOutOfStock, status)
	}

	item := Item{
		OrderID: orderID, ProductID: productID, VariantID: variantID,
		NameSnapshot: name, Quantity: quantity, Preference: preference,
	}
	if item.UnitPrice, err = money.New(priceMinor, currency); err != nil {
		return Item{}, err
	}
	if item.Preference == "" {
		item.Preference = PreferAsk
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO order_items
		   (order_id, product_id, variant_id, name_snapshot, unit_price_minor, quantity, substitution_preference)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text, created_at`,
		orderID, productID, nullableUUID(variantID), name, priceMinor, quantity, item.Preference).
		Scan(&item.ID, &item.CreatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	if err := recomputeTotal(ctx, tx, orderID); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("commit: %w", err)
	}
	return item, nil
}

func recomputeTotal(ctx context.Context, tx pgx.Tx, orderID string) error {
	// Summed in the database from the stored line snapshots, so the total and
	// the lines cannot disagree.
	_, err := tx.Exec(ctx,
		`UPDATE orders SET items_total_minor = COALESCE((
		   SELECT sum(unit_price_minor * quantity) FROM order_items
		    WHERE order_id = $1 AND status NOT IN ('REMOVED', 'UNAVAILABLE')), 0),
		   updated_at = now()
		 WHERE id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("recompute order total: %w", err)
	}
	return nil
}

// Place moves a cart to PLACED and sets the acceptance deadline.
//
// It refuses when no timeout is configured. BD-12 is unresolved, and the
// business decision register asks for "an explicit unset state that fails
// loudly rather than defaulting silently" — a guessed timeout would either
// auto-cancel orders merchants were about to accept, or leave customers
// waiting indefinitely.
func (s *Store) Place(ctx context.Context, orderID string, now time.Time) (Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var merchantID string
	var itemCount int
	if err := tx.QueryRow(ctx,
		`SELECT o.merchant_id::text, (SELECT count(*) FROM order_items i WHERE i.order_id = o.id)
		   FROM orders o WHERE o.id = $1`, orderID).Scan(&merchantID, &itemCount); err != nil {
		return Order{}, fmt.Errorf("load order: %w", err)
	}
	if itemCount == 0 {
		return Order{}, errors.New("merchant: an empty cart cannot be placed")
	}

	var timeoutSeconds *int
	if err := tx.QueryRow(ctx,
		`SELECT accept_timeout_seconds FROM merchant_config WHERE merchant_id = $1`, merchantID).
		Scan(&timeoutSeconds); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("load merchant config: %w", err)
	}
	if timeoutSeconds == nil {
		return Order{}, ErrAcceptTimeoutUnset
	}
	deadline := now.Add(time.Duration(*timeoutSeconds) * time.Second)

	order, err := scanOrder(tx.QueryRow(ctx,
		`UPDATE orders SET status = 'PLACED', accept_deadline = $2, updated_at = now()
		  WHERE id = $1 AND status = 'CART' RETURNING `+orderColumns,
		orderID, deadline))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrStale
	}
	if err != nil {
		return Order{}, fmt.Errorf("place order: %w", err)
	}
	if err := appendOrderHistory(ctx, tx, orderID, StatusCart, StatusPlaced, "CUSTOMER", order.CustomerUserID, nil); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit: %w", err)
	}
	return order, nil
}

// Transition moves an order, compare-and-set on its current status.
func (s *Store) Transition(ctx context.Context, orderID string, from, to OrderStatus, actorType, actorID string, metadata map[string]any) (Order, error) {
	if err := Machine.Validate(from, to); err != nil {
		return Order{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The merchant timestamps document 072 requires, set by the transition
	// that earns them rather than by a separate call that could be missed.
	var timestampColumn string
	switch to {
	case StatusConfirmed:
		timestampColumn = ", accepted_at = COALESCE(accepted_at, now())"
	case StatusPreparing:
		timestampColumn = ", preparation_started_at = COALESCE(preparation_started_at, now())"
	case StatusReadyForPickup:
		timestampColumn = ", ready_at = COALESCE(ready_at, now())"
	}

	order, err := scanOrder(tx.QueryRow(ctx,
		`UPDATE orders SET status = $3, updated_at = now()`+timestampColumn+`
		  WHERE id = $1 AND status = $2 RETURNING `+orderColumns,
		orderID, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("%w: expected %s", ErrStale, from)
	}
	if err != nil {
		return Order{}, fmt.Errorf("transition order: %w", err)
	}
	if err := appendOrderHistory(ctx, tx, orderID, from, to, actorType, actorID, metadata); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit: %w", err)
	}
	return order, nil
}

// Reject records a merchant rejection with its reason.
func (s *Store) Reject(ctx context.Context, orderID string, from OrderStatus, actorID, reason string) (Order, error) {
	if !MerchantCancellable(from) {
		return Order{}, fmt.Errorf("%w: an order that is %s cannot be rejected", ErrNotCancellable, from)
	}
	order, err := s.Transition(ctx, orderID, from, StatusCancelled, "MERCHANT", actorID,
		map[string]any{"reason": reason, "rejected_by": "merchant"})
	if err != nil {
		return Order{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE orders SET rejection_reason = $2 WHERE id = $1`, orderID, reason); err != nil {
		return Order{}, fmt.Errorf("record rejection reason: %w", err)
	}
	return order, nil
}

// SweepAcceptTimeouts cancels orders no merchant answered in time.
//
// The deadline is per order and was set from configuration at placement, so
// this sweep enforces the merchant's own configured timeout and never a
// default of its own.
func (s *Store) SweepAcceptTimeouts(ctx context.Context, now time.Time) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`UPDATE orders SET status = 'CANCELLED', cancelled_reason = 'merchant did not respond', updated_at = now()
		  WHERE status = 'PLACED' AND accept_deadline IS NOT NULL AND accept_deadline <= $1
		  RETURNING id::text`, now)
	if err != nil {
		return 0, fmt.Errorf("sweep acceptance timeouts: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		if err := appendOrderHistory(ctx, tx, id, StatusPlaced, StatusCancelled, "SYSTEM", "",
			map[string]any{"reason": "acceptance timeout"}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return int64(len(ids)), nil
}

// OrderByID loads an order and its items.
func (s *Store) OrderByID(ctx context.Context, id string) (Order, error) {
	order, err := scanOrder(s.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("load order: %w", err)
	}
	if order.Items, err = s.ItemsOf(ctx, id, order.ItemsTotal.Currency); err != nil {
		return Order{}, err
	}
	return order, nil
}

// ItemsOf returns an order's lines.
func (s *Store) ItemsOf(ctx context.Context, orderID string, currency money.Currency) ([]Item, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, order_id::text, COALESCE(product_id::text, ''), COALESCE(variant_id::text, ''),
		        name_snapshot, unit_price_minor, quantity, substitution_preference, status, created_at
		   FROM order_items WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load order items: %w", err)
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		var item Item
		var priceMinor int64
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.VariantID,
			&item.NameSnapshot, &priceMinor, &item.Quantity, &item.Preference,
			&item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		if item.UnitPrice, err = money.New(priceMinor, currency); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// --- item issues (document 74) -----------------------------------------------

// RecordIssue stores an item problem and its resolution.
//
// The original order line is never mutated (document 74). Its status changes to
// SUBSTITUTED or REMOVED, and the substitute lives in the issue row — so what
// the customer ordered stays readable after what they received changed.
func (s *Store) RecordIssue(ctx context.Context, issue Issue, itemStatus string) (Issue, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Issue{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var substituteName, substitutePrice, priceDifference any
	if issue.SubstituteName != "" {
		substituteName = issue.SubstituteName
	}
	if issue.SubstitutePrice != nil {
		substitutePrice = issue.SubstitutePrice.Minor
	}
	if issue.PriceDifference != nil {
		priceDifference = issue.PriceDifference.Minor
	}
	resolution := issue.Resolution
	if resolution == "" {
		resolution = "PENDING"
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO order_item_issues
		   (order_id, order_item_id, reason, action, substitute_name,
		    substitute_unit_price_minor, price_difference_minor, resolution)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text, created_at`,
		issue.OrderID, issue.OrderItemID, issue.Reason, issue.Action,
		substituteName, substitutePrice, priceDifference, resolution).
		Scan(&issue.ID, &issue.CreatedAt)
	if err != nil {
		return Issue{}, fmt.Errorf("record item issue: %w", err)
	}

	if itemStatus != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE order_items SET status = $2 WHERE id = $1`, issue.OrderItemID, itemStatus); err != nil {
			return Issue{}, fmt.Errorf("update item status: %w", err)
		}
		if err := recomputeTotal(ctx, tx, issue.OrderID); err != nil {
			return Issue{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Issue{}, fmt.Errorf("commit: %w", err)
	}
	issue.Resolution = resolution
	return issue, nil
}

// IssuesOf returns an order's recorded item problems.
func (s *Store) IssuesOf(ctx context.Context, orderID string) ([]Issue, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, order_id::text, order_item_id::text, reason, action,
		        COALESCE(substitute_name, ''), resolution, created_at
		   FROM order_item_issues WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load item issues: %w", err)
	}
	defer rows.Close()

	var out []Issue
	for rows.Next() {
		var issue Issue
		if err := rows.Scan(&issue.ID, &issue.OrderID, &issue.OrderItemID, &issue.Reason,
			&issue.Action, &issue.SubstituteName, &issue.Resolution, &issue.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, issue)
	}
	return out, rows.Err()
}

// --- inventory (document 69) -------------------------------------------------

// Reserve holds stock for an order, atomically.
//
// Document 69: "Atomic reservation is required so two customers cannot safely
// reserve the same final unit simultaneously." The predicate does the work —
// a check followed by an update has a window between them, and the last unit
// of anything is precisely where two customers meet.
func (s *Store) Reserve(ctx context.Context, storeID, productID, variantID string, quantity int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE inventory
		    SET reserved_quantity = reserved_quantity + $4, updated_at = now()
		  WHERE store_id = $1 AND product_id = $2
		    AND variant_id IS NOT DISTINCT FROM $3
		    AND available
		    AND (quantity IS NULL OR reserved_quantity + $4 <= quantity)`,
		storeID, productID, nullableUUID(variantID), quantity)
	if err != nil {
		return fmt.Errorf("reserve inventory: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrOutOfStock
	}
	return nil
}

// ReleaseReservation returns stock when an order is cancelled or fails.
func (s *Store) ReleaseReservation(ctx context.Context, storeID, productID, variantID string, quantity int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE inventory
		    SET reserved_quantity = GREATEST(0, reserved_quantity - $4), updated_at = now()
		  WHERE store_id = $1 AND product_id = $2 AND variant_id IS NOT DISTINCT FROM $3`,
		storeID, productID, nullableUUID(variantID), quantity)
	if err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}
	return nil
}

// SetInventory writes a stock level.
func (s *Store) SetInventory(ctx context.Context, storeID, productID, variantID string, available bool, quantity *int) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO inventory (store_id, product_id, variant_id, available, quantity)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (store_id, product_id, variant_id) DO UPDATE
		   SET available = EXCLUDED.available, quantity = EXCLUDED.quantity, updated_at = now()`,
		storeID, productID, nullableUUID(variantID), available, quantity)
	if err != nil {
		return fmt.Errorf("set inventory: %w", err)
	}
	return nil
}

func appendOrderHistory(ctx context.Context, tx pgx.Tx, orderID string, from, to OrderStatus, actorType, actorID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode order history metadata: %w", err)
	}
	var fromStatus, actor any
	if from != "" {
		fromStatus = from
	}
	if actorID != "" {
		actor = actorID
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO order_status_history (order_id, from_status, to_status, actor_type, actor_id, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		orderID, fromStatus, to, actorType, actor, encoded); err != nil {
		return fmt.Errorf("append order history: %w", err)
	}
	return nil
}

func nullableUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
