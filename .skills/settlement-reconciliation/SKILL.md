---
name: settlement-reconciliation
description: Driver payouts, merchant settlement, COD handling, and reconciling internal records against providers, banks, and cash. Use for payout runs, settlement cycles, or investigating a financial discrepancy.
---

# Purpose

Move money out correctly and prove the internal record matches the outside world.

# When to Use

Driver payouts, merchant settlement, COD reconciliation, provider/bank reconciliation, financial discrepancy investigation.

# Rules

- **Settlement amounts derive from ledger entries** (`financial-ledger`), never from a recomputation done at payout time. Two independent calculations will eventually disagree, and then nobody knows which is right.
- **Reconcile against three external realities** (`docs/19`): payment provider records, bank/Raast records, and cash records. Internal consistency alone proves nothing.
- **A discrepancy is an alert, not a rounding tolerance.** Investigate; do not auto-adjust to make totals match. Auto-correction hides the bug that caused the gap.
- **Payout execution is idempotent.** A retried payout run must not pay twice — this is the highest-consequence duplicate in the system.
- COD creates a real cash liability: driver collects, platform records, merchant balance updates, settlement clears (`docs/19`, `docs/56`). Cash in a driver's hand is platform-owed money and must be tracked as such.
- Settlement runs produce an auditable artifact: what was paid, to whom, from which entries.
- Failed or partial payouts have a defined recovery path — never a silent retry loop.

# Workflow

1. Select the settlement window and the entries within it.
2. Aggregate per party from the ledger.
3. Produce the settlement record before executing payment.
4. Execute through the payment adapter with an idempotency key.
5. Record the result, including partial and failed outcomes.
6. Reconcile against provider, bank, and cash records; alert on any gap.

# Verification

**Always Level 5.**

Required: settlement total equals the ledger sum for the window, retried payout run pays once, partial payout failure recovers correctly, COD collected but delivery failed, entry arriving during a settlement run is not double-counted, deliberate discrepancy is detected and alerts rather than self-corrects.

# Blocking Conditions

- Settlement cadence, minimum payout threshold, or fee handling undocumented → `BLOCKED_TASKS.md`.
- Discrepancy tolerance and escalation path undefined → ask; do not pick a tolerance.
- COD liability allocation between driver, merchant, and platform unspecified → financial decision.
- No provider sandbox for payouts → block rather than test in production.

# Relevant Documentation

`docs/19-payment-wallet-settlement.md` · `docs/55-driver-payouts.md` · `docs/56-merchant-settlement-and-cod.md` · `docs/58-payment-webhooks-and-reconciliation.md` · `docs/236-refunds-settlements-and-reconciliation.md` · `docs/459-driver-earnings-engine.md` · `docs/516-settlement-model.md` · `docs/517-reconciliation-engine.md` · `docs/518-cash-payment-model.md`
