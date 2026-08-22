# 57 — Refunds, Chargebacks & Disputes

## Objective
Handle financial reversals without editing historical transactions.

## Refund Types
```text
FULL
PARTIAL
CANCELLATION
SERVICE_FAILURE
SUPPORT_ADJUSTMENT
```

## Refund Flow
```text
Dispute
 ↓
Eligibility
 ↓
Decision
 ↓
Refund Request
 ↓
Provider
 ↓
Ledger Adjustment
```

## Chargebacks
Provider-originated chargebacks must create an internal case.

Track:
- provider case ID
- payment
- amount
- status
- evidence
- deadlines

## Driver Earnings Impact
If a completed job is refunded, driver earnings policy must explicitly define whether earnings:
- remain
- are partially adjusted
- are reversed

Never silently modify a previous earnings record.

## Dispute States
```text
OPEN
UNDER_REVIEW
RESOLVED
REJECTED
ESCALATED
```

## Definition of Done
Refunds, disputes and chargebacks produce traceable financial adjustments.
