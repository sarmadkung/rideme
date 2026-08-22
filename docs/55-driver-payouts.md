# 55 — Driver Payouts

## Objective
Move available driver earnings to a verified payout destination.

## Payout Flow
```text
Available Earnings
 ↓
Payout Request
 ↓
Risk / Eligibility Checks
 ↓
Payout Provider
 ↓
Processing
 ↓
Completed / Failed
```

## Payout States
```text
REQUESTED
PROCESSING
COMPLETED
FAILED
CANCELLED
```

## Eligibility
Check:
- available balance
- verified identity
- payout account
- minimum payout threshold
- pending disputes
- risk restrictions

## Payout Destination
Store provider references or tokenized identifiers rather than unnecessary sensitive financial data.

## Idempotency
A payout request must have a unique idempotency key.

## Failure
A failed payout must not destroy the driver's balance.

It remains available or moves to a controlled retry state.

## Definition of Done
Drivers can request payouts and every payout can be reconciled against the ledger.
