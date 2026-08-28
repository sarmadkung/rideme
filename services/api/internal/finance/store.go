package finance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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

// --- ledger ------------------------------------------------------------------

// Post writes a balanced transaction.
//
// Balance is checked in Go, then the entries are written in one transaction and
// re-summed in SQL before the commit. Checking twice is deliberate: the Go
// check catches the bug, and the SQL check catches the case where the entries
// written differ from the entries built.
func (s *Store) Post(ctx context.Context, t Transaction) (Transaction, error) {
	if err := t.Balance(); err != nil {
		return Transaction{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var jobID, orderID, intentID, reverses, key any
	if t.JobID != "" {
		jobID = t.JobID
	}
	if t.OrderID != "" {
		orderID = t.OrderID
	}
	if t.IntentID != "" {
		intentID = t.IntentID
	}
	if t.ReversesID != "" {
		reverses = t.ReversesID
	}
	if t.IdempotencyKey != "" {
		key = t.IdempotencyKey
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO ledger_transactions (kind, job_id, order_id, intent_id, reverses_id, idempotency_key, description)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text, created_at`,
		t.Kind, jobID, orderID, intentID, reverses, key, t.Description).
		Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		if isUnique(err) {
			// The same idempotency key already posted this movement. Return
			// the original rather than posting it twice.
			return s.transactionByKey(ctx, t.IdempotencyKey)
		}
		return Transaction{}, fmt.Errorf("insert ledger transaction: %w", err)
	}

	for i, entry := range t.Entries {
		var subjectType, subjectID any
		if entry.SubjectType != "" {
			subjectType = entry.SubjectType
		}
		if entry.SubjectID != "" {
			subjectID = entry.SubjectID
		}
		err := tx.QueryRow(ctx,
			`INSERT INTO ledger_entries (transaction_id, account, amount_minor, currency, subject_type, subject_id)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			t.ID, entry.Account, entry.Amount.Minor, entry.Amount.Currency, subjectType, subjectID).
			Scan(&t.Entries[i].ID)
		if err != nil {
			return Transaction{}, fmt.Errorf("insert ledger entry: %w", err)
		}
		t.Entries[i].TransactionID = t.ID
	}

	// The second check, on what was actually written.
	var written int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(sum(amount_minor), 0) FROM ledger_entries WHERE transaction_id = $1`,
		t.ID).Scan(&written); err != nil {
		return Transaction{}, fmt.Errorf("verify balance: %w", err)
	}
	if written != 0 {
		return Transaction{}, fmt.Errorf("%w: stored entries sum to %d", ErrUnbalanced, written)
	}

	if err := tx.Commit(ctx); err != nil {
		return Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return t, nil
}

func (s *Store) transactionByKey(ctx context.Context, key string) (Transaction, error) {
	var t Transaction
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, kind, COALESCE(job_id::text,''), COALESCE(intent_id::text,''),
		        COALESCE(description,''), created_at
		   FROM ledger_transactions WHERE idempotency_key = $1`, key).
		Scan(&t.ID, &t.Kind, &t.JobID, &t.IntentID, &t.Description, &t.CreatedAt)
	if err != nil {
		return Transaction{}, fmt.Errorf("load transaction by key: %w", err)
	}
	if t.Entries, err = s.EntriesOf(ctx, t.ID); err != nil {
		return Transaction{}, err
	}
	return t, nil
}

// EntriesOf returns a transaction's entries.
func (s *Store) EntriesOf(ctx context.Context, transactionID string) ([]Entry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, transaction_id::text, account, amount_minor, currency,
		        COALESCE(subject_type, ''), COALESCE(subject_id::text, ''), created_at
		   FROM ledger_entries WHERE transaction_id = $1 ORDER BY id`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var minor int64
		var currency money.Currency
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.Account, &minor, &currency,
			&e.SubjectType, &e.SubjectID, &e.CreatedAt); err != nil {
			return nil, err
		}
		if e.Amount, err = money.New(minor, currency); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Balance derives an account balance from the entries.
//
// Document 53: "Balances should be derived from immutable ledger entries."
// Derived, not stored — a cached balance is a second source of truth that can
// disagree with the first, and reconciling them is exactly the problem the
// ledger exists to avoid.
func (s *Store) Balance(ctx context.Context, account Account, subjectType SubjectType, subjectID string) (money.Amount, error) {
	var minor int64
	query := `SELECT COALESCE(sum(amount_minor), 0) FROM ledger_entries WHERE account = $1`
	args := []any{account}
	if subjectID != "" {
		query += ` AND subject_type = $2 AND subject_id = $3`
		args = append(args, subjectType, subjectID)
	}
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&minor); err != nil {
		return money.Amount{}, fmt.Errorf("compute balance: %w", err)
	}
	return money.New(minor, money.PKR)
}

// LedgerBalances reports whether the whole ledger sums to zero.
//
// The single query that proves the books are consistent. If this is ever
// non-zero, something wrote entries outside Post.
func (s *Store) LedgerBalances(ctx context.Context) (bool, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(sum(amount_minor), 0) FROM ledger_entries`).Scan(&total); err != nil {
		return false, 0, fmt.Errorf("check ledger balance: %w", err)
	}
	return total == 0, total, nil
}

// UnbalancedTransactions lists any transaction whose entries do not sum to
// zero — the reconciliation query an operator runs when something looks wrong.
func (s *Store) UnbalancedTransactions(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT transaction_id::text FROM ledger_entries
		  GROUP BY transaction_id HAVING sum(amount_minor) <> 0`)
	if err != nil {
		return nil, fmt.Errorf("find unbalanced transactions: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// --- payment intents ---------------------------------------------------------

// Intent is a payment in progress (document 52).
type Intent struct {
	ID                string
	JobID             string
	OrderID           string
	CustomerUserID    string
	Amount            money.Amount
	Status            string
	Method            string
	Provider          string
	ProviderReference string
	Captured          money.Amount
	Refunded          money.Amount
	CreatedAt         time.Time
}

const intentColumns = `id::text, COALESCE(job_id::text,''), COALESCE(order_id::text,''),
	customer_user_id::text, amount_minor, currency, status, method, provider,
	COALESCE(provider_reference,''), captured_minor, refunded_minor, created_at`

func scanIntent(row pgx.Row) (Intent, error) {
	var i Intent
	var amount, captured, refunded int64
	var currency money.Currency
	err := row.Scan(&i.ID, &i.JobID, &i.OrderID, &i.CustomerUserID, &amount, &currency,
		&i.Status, &i.Method, &i.Provider, &i.ProviderReference, &captured, &refunded, &i.CreatedAt)
	if err != nil {
		return Intent{}, err
	}
	if i.Amount, err = money.New(amount, currency); err != nil {
		return Intent{}, err
	}
	if i.Captured, err = money.New(captured, currency); err != nil {
		return Intent{}, err
	}
	i.Refunded, err = money.New(refunded, currency)
	return i, err
}

// CreateIntent opens a payment intent, idempotently.
func (s *Store) CreateIntent(ctx context.Context, i Intent, idempotencyKey string) (Intent, error) {
	var jobID, orderID, key any
	if i.JobID != "" {
		jobID = i.JobID
	}
	if i.OrderID != "" {
		orderID = i.OrderID
	}
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	provider := i.Provider
	if provider == "" {
		provider = "none"
	}

	created, err := scanIntent(s.pool.QueryRow(ctx,
		`INSERT INTO payment_intents (job_id, order_id, customer_user_id, amount_minor, currency, method, provider, idempotency_key)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+intentColumns,
		jobID, orderID, i.CustomerUserID, i.Amount.Minor, i.Amount.Currency, i.Method, provider, key))
	if err != nil {
		if isUnique(err) {
			// Either the idempotency key or the one-live-intent-per-job index.
			// Both mean "this is already being paid for".
			if idempotencyKey != "" {
				return s.intentByKey(ctx, i.CustomerUserID, idempotencyKey)
			}
			return Intent{}, ErrAlreadyCaptured
		}
		return Intent{}, fmt.Errorf("create payment intent: %w", err)
	}
	return created, nil
}

func (s *Store) intentByKey(ctx context.Context, customerID, key string) (Intent, error) {
	intent, err := scanIntent(s.pool.QueryRow(ctx,
		`SELECT `+intentColumns+` FROM payment_intents
		  WHERE customer_user_id = $1 AND idempotency_key = $2`, customerID, key))
	if err != nil {
		return Intent{}, fmt.Errorf("load intent by key: %w", err)
	}
	return intent, nil
}

// IntentByID loads an intent.
func (s *Store) IntentByID(ctx context.Context, id string) (Intent, error) {
	intent, err := scanIntent(s.pool.QueryRow(ctx, `SELECT `+intentColumns+` FROM payment_intents WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, ErrNotFound
	}
	if err != nil {
		return Intent{}, fmt.Errorf("load intent: %w", err)
	}
	return intent, nil
}

// Capture records a successful payment and posts the ledger movement, in one
// transaction.
//
// The status predicate makes capture exactly-once: a webhook replayed by the
// provider, or two workers processing the same event, produce one capture and
// one ledger transaction. Document 59 names "two captures" as a race to guard.
func (s *Store) Capture(ctx context.Context, intentID string, amount money.Amount) (Intent, Transaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intent, err := scanIntent(tx.QueryRow(ctx,
		`UPDATE payment_intents
		    SET captured_minor = $2, status = 'CAPTURED', updated_at = now()
		  WHERE id = $1 AND status IN ('REQUIRES_PAYMENT', 'PROCESSING', 'AUTHORIZED')
		  RETURNING `+intentColumns,
		intentID, amount.Minor))
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, Transaction{}, ErrAlreadyCaptured
	}
	if err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("capture intent: %w", err)
	}

	movement, err := CustomerPayment(amount, intent.CustomerUserID, intent.JobID, intent.ID)
	if err != nil {
		return Intent{}, Transaction{}, err
	}
	posted, err := postWithin(ctx, tx, movement)
	if err != nil {
		return Intent{}, Transaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return intent, posted, nil
}

// Refund records a refund and posts its ledger movement.
//
// The bound is in the predicate, not only in a prior read: a refund can never
// exceed what remains captured, however many refund requests arrive at once.
func (s *Store) Refund(ctx context.Context, intentID string, amount money.Amount) (Intent, Transaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intent, err := scanIntent(tx.QueryRow(ctx,
		`UPDATE payment_intents
		    SET refunded_minor = refunded_minor + $2,
		        status = CASE WHEN refunded_minor + $2 >= captured_minor THEN 'REFUNDED'
		                      ELSE 'PARTIALLY_REFUNDED' END,
		        updated_at = now()
		  WHERE id = $1
		    AND status IN ('CAPTURED', 'PARTIALLY_REFUNDED')
		    AND refunded_minor + $2 <= captured_minor
		  RETURNING `+intentColumns,
		intentID, amount.Minor))
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, Transaction{}, ErrRefundExceeds
	}
	if err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("refund intent: %w", err)
	}

	movement, err := Refund(amount, intent.CustomerUserID, intent.JobID, intent.ID)
	if err != nil {
		return Intent{}, Transaction{}, err
	}
	posted, err := postWithin(ctx, tx, movement)
	if err != nil {
		return Intent{}, Transaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Intent{}, Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return intent, posted, nil
}

// postWithin writes a transaction inside a caller's transaction, so the
// payment state change and its ledger movement commit together. A captured
// payment with no ledger entry is money the books do not know about.
func postWithin(ctx context.Context, tx pgx.Tx, t Transaction) (Transaction, error) {
	if err := t.Balance(); err != nil {
		return Transaction{}, err
	}
	var jobID, intentID any
	if t.JobID != "" {
		jobID = t.JobID
	}
	if t.IntentID != "" {
		intentID = t.IntentID
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO ledger_transactions (kind, job_id, intent_id, description)
		 VALUES ($1,$2,$3,$4) RETURNING id::text, created_at`,
		t.Kind, jobID, intentID, t.Description).Scan(&t.ID, &t.CreatedAt); err != nil {
		return Transaction{}, fmt.Errorf("insert ledger transaction: %w", err)
	}
	for i, entry := range t.Entries {
		var subjectType, subjectID any
		if entry.SubjectType != "" {
			subjectType = entry.SubjectType
		}
		if entry.SubjectID != "" {
			subjectID = entry.SubjectID
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO ledger_entries (transaction_id, account, amount_minor, currency, subject_type, subject_id)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			t.ID, entry.Account, entry.Amount.Minor, entry.Amount.Currency, subjectType, subjectID).
			Scan(&t.Entries[i].ID); err != nil {
			return Transaction{}, fmt.Errorf("insert ledger entry: %w", err)
		}
	}
	return t, nil
}

// --- webhooks (document 58) --------------------------------------------------

// VerifySignature checks an HMAC-SHA256 provider signature in constant time.
//
// Document 58: "verify signatures" and "do not trust client-side payment
// success". An unverified webhook is an endpoint anyone can use to mark
// anything paid.
func VerifySignature(secret string, payload []byte, signature string) error {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return ErrBadSignature
	}
	return nil
}

// SignForTest produces the signature a provider would send. It exists so tests
// exercise the real verification path rather than a reimplementation of it.
func SignForTest(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// RecordWebhook stores a provider event, reporting whether it is new.
//
// Deduplication by provider event id (documents 52, 58). A provider replaying
// a capture must not capture twice, and the dedup must be durable — an
// in-memory set forgets on restart, which is when replays are most likely.
func (s *Store) RecordWebhook(ctx context.Context, provider, eventID, eventType string, payload []byte, signatureOK bool, intentID string) (isNew bool, err error) {
	var intent any
	if intentID != "" {
		intent = intentID
	}
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO payment_webhook_events (provider, event_id, intent_id, event_type, payload, signature_ok)
		 VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (provider, event_id) DO NOTHING`,
		provider, eventID, intent, eventType, json.RawMessage(payload), signatureOK)
	if err != nil {
		return false, fmt.Errorf("record webhook: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// MarkWebhookProcessed records that an event has been handled.
func (s *Store) MarkWebhookProcessed(ctx context.Context, provider, eventID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE payment_webhook_events SET processed_at = now()
		  WHERE provider = $1 AND event_id = $2 AND processed_at IS NULL`, provider, eventID)
	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}
	return nil
}

// --- commission configuration ------------------------------------------------

// CommissionRateFor loads the configured rate.
//
// BD-05 is unresolved and the table ships empty, so this returns
// ErrNoCommission rather than a default. Earnings cannot be computed until
// someone sets a rate, which is correct: a guessed rate pays every driver the
// wrong amount, retroactively.
func (s *Store) CommissionRateFor(ctx context.Context, jobType string, subject SubjectType) (CommissionRate, error) {
	var rate CommissionRate
	var flatMinor int64
	err := s.pool.QueryRow(ctx,
		`SELECT job_type, subject_type, rate_bps, flat_minor, version
		   FROM commission_rates
		  WHERE job_type = $1 AND subject_type = $2
		    AND active_from <= now() AND (active_to IS NULL OR active_to > now())
		  ORDER BY version DESC LIMIT 1`, jobType, subject).
		Scan(&rate.JobType, &rate.SubjectType, &rate.RateBPS, &flatMinor, &rate.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommissionRate{}, ErrNoCommission
	}
	if err != nil {
		return CommissionRate{}, fmt.Errorf("load commission rate: %w", err)
	}
	if rate.Flat, err = money.New(flatMinor, money.PKR); err != nil {
		return CommissionRate{}, err
	}
	return rate, nil
}

// SetCommissionRate stores a rate.
func (s *Store) SetCommissionRate(ctx context.Context, rate CommissionRate) error {
	flat := int64(0)
	if !rate.Flat.IsZero() {
		flat = rate.Flat.Minor
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO commission_rates (job_type, subject_type, rate_bps, flat_minor, version)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (job_type, subject_type, version) DO UPDATE
		   SET rate_bps = EXCLUDED.rate_bps, flat_minor = EXCLUDED.flat_minor`,
		rate.JobType, rate.SubjectType, rate.RateBPS, flat, rate.Version)
	if err != nil {
		return fmt.Errorf("set commission rate: %w", err)
	}
	return nil
}

// --- payouts and reconciliation ----------------------------------------------

// CreatePayout opens a settlement for a period.
//
// One live payout per subject per period, enforced by a partial unique index.
// Document 59 names "two payout requests" as a race to guard, and paying a
// driver twice for one week is discovered by whoever reconciles the bank.
func (s *Store) CreatePayout(ctx context.Context, subjectType SubjectType, subjectID string, amount money.Amount, periodStart, periodEnd time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO payouts (subject_type, subject_id, amount_minor, currency, period_start, period_end)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		subjectType, subjectID, amount.Minor, amount.Currency, periodStart, periodEnd).Scan(&id)
	if err != nil {
		if isUnique(err) {
			return "", errors.New("finance: a payout already exists for this subject and period")
		}
		return "", fmt.Errorf("create payout: %w", err)
	}
	return id, nil
}

// OpenReconciliationCase records a mismatch for investigation.
//
// Document 58: "Create reconciliation cases rather than silently fixing
// mismatches." BD-08's recommended default is zero tolerance — any discrepancy
// raises a case and nothing auto-adjusts, because a tolerance band hides the
// bug that caused the gap.
func (s *Store) OpenReconciliationCase(ctx context.Context, provider, kind, intentID, providerRef string, expected, actual *money.Amount, detail map[string]any) (string, error) {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("encode reconciliation detail: %w", err)
	}
	var intent, ref, expectedMinor, actualMinor any
	if intentID != "" {
		intent = intentID
	}
	if providerRef != "" {
		ref = providerRef
	}
	if expected != nil {
		expectedMinor = expected.Minor
	}
	if actual != nil {
		actualMinor = actual.Minor
	}
	var id string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO reconciliation_cases (provider, kind, intent_id, provider_reference, expected_minor, actual_minor, detail)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		provider, kind, intent, ref, expectedMinor, actualMinor, encoded).Scan(&id); err != nil {
		return "", fmt.Errorf("open reconciliation case: %w", err)
	}
	return id, nil
}

// OpenCases counts unresolved reconciliation cases.
func (s *Store) OpenCases(ctx context.Context) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM reconciliation_cases WHERE status = 'OPEN'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count open cases: %w", err)
	}
	return count, nil
}

func isUnique(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
