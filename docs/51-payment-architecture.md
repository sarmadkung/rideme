# 51 — Payment Architecture

## Objective
Build a payment system supporting customer payments, refunds, driver earnings, merchant settlements and COD without mixing operational jobs with financial state.

## Principle
The payment/ledger system is the financial source of truth.

A job may reference payments, but job status must not be used as a financial ledger.

## Components
```text
Payment Intent
Payment Method
Payment Transaction
Refund
Wallet
Ledger
Driver Earnings
Merchant Settlement
Payout
```

## Flow
```text
Job Confirmed
  ↓
Payment Intent
  ↓
Authorize / Collect
  ↓
Job Completed
  ↓
Capture / Finalize
  ↓
Ledger
  ↓
Earnings / Settlement
```

## Payment Providers
Create an abstraction:
```text
PaymentProvider
```

Provider-specific code must not leak into domain logic.

## Payment States
```text
CREATED
REQUIRES_ACTION
AUTHORIZED
CAPTURED
FAILED
CANCELLED
REFUNDED
PARTIALLY_REFUNDED
```

## Idempotency
All money-moving operations require idempotency keys.

## Currency
Store:
- integer minor units
- ISO currency
- exact transaction amount

Never use floating-point values for money.

## Definition of Done
- payment provider abstraction exists
- payment state is independent from job state
- money uses integer minor units
- idempotency is enforced
- every financial operation produces auditable records
