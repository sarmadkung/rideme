---
name: financial-ledger
description: The append-only ledger that is the single authoritative record of money on the platform. Use whenever earnings, fees, commissions, adjustments, or balances are involved — and before any code computes a financial figure outside the ledger.
---

# Purpose

Keep one auditable financial truth that can be reconstructed and never silently rewritten.

# When to Use

Driver earnings, platform fees, commissions, compensation, adjustments, wallet balances, any displayed money figure.

# Rules

- **The ledger is append-only** (`docs/19`). Never `UPDATE` or `DELETE` a historical entry. A correction is a new, linked entry. This is what makes the financial history auditable, and it is not negotiable for convenience.
- **The ledger is the single source of financial truth.** Balances are derived from entries — never stored as an independently mutable number that code can drift from the entries.
- **Never compute financial truth in a client.** Mobile apps and dashboards display server-provided figures. Two clients computing a fee independently will disagree, and the customer will see the disagreement.
- **Every entry is attributable**: what job or event caused it, which party it belongs to, when, and why.
- Entries are written inside the transaction that confirms the operational event, or by an idempotent consumer of it — never fire-and-forget.
- Writing ledger entries is idempotent: replaying an event must not double-credit.

# Shape (`docs/19`)

```text
Driver earning       +500
Platform fee          -50
Compensation         +100
Settlement           -550
```

Entries net to zero across parties for a completed transaction — that property is what makes reconciliation possible (`settlement-reconciliation`).

# Workflow

1. Identify the operational event that has financial meaning (job completed, refund confirmed, adjustment approved).
2. Determine every entry it produces, for every party.
3. Write them atomically with a natural idempotency key derived from the event.
4. Derive balances by aggregation; never mutate a stored total.
5. Confirm the entries net correctly before considering it done.

# Verification

**Always Level 5.**

Required: entries net to zero for a completed job, replayed event does not double-credit, correction entry leaves the original intact, derived balance matches the entry sum exactly, concurrent writes for one job produce one consistent set, no code path updates or deletes an existing entry.

Add a database-level guarantee against mutation where possible (`database-architecture`) — a test alone is weaker than a constraint.

# Blocking Conditions

- Commission percentage or fee structure undocumented → `BLOCKED_TASKS.md`; commercial decision.
- Rounding and currency precision rules unspecified → ask (`docs/451`). Rounding chosen ad hoc creates reconciliation drift that is painful to unwind later.
- An operational event's financial consequence is undefined → do not invent entries.

# Relevant Documentation

`docs/19-payment-wallet-settlement.md` · `docs/53-wallets-and-ledger.md` · `docs/54-driver-earnings-and-commission.md` · `docs/63-financial-data-model.md` · `docs/235-ledger-and-accounting.md` · `docs/451-money-and-currency-standard.md` · `docs/480-financial-data-model.md` · `docs/510-commission-engine.md` · `docs/515-ledger-model.md`
