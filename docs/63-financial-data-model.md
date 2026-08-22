# 63 — Financial Data Model

## Objective
Define the durable financial entities before implementation.

## Core Tables
```text
payment_methods
payment_intents
payment_transactions
payment_webhook_events
refunds
ledger_accounts
ledger_transactions
ledger_entries
driver_earnings
wallets
payout_accounts
payouts
merchant_settlements
cod_collections
disputes
reconciliation_cases
```

## Key Relationships
```text
Job
 └── Payment Intent
      └── Payment Transactions
           └── Ledger Transactions

Job
 └── Driver Earnings
      └── Driver Payable Account

Merchant Order
 └── Settlement
```

## Constraints
Use:
- unique provider references
- unique idempotency keys
- foreign keys
- immutable ledger entries
- currency consistency

## Money
```text
amount_minor BIGINT
currency CHAR(3)
```

Never use floating point.

## Audit
Financial records require created timestamps, actor/reference information and immutable history.

## Definition of Done
The schema supports payment, earnings, settlement, refund, payout and reconciliation without relying on mutable job state.
