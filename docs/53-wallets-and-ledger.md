# 53 — Wallets & Double-Entry Ledger

## Objective
Represent financial balances and movements correctly.

## Ledger Principle
Balances should be derived from immutable ledger entries or maintained as a cached balance backed by the ledger.

Never mutate historical financial entries.

## Accounts
Examples:
```text
CUSTOMER_RECEIVABLE
CUSTOMER_WALLET
PLATFORM_REVENUE
DRIVER_PAYABLE
MERCHANT_PAYABLE
TAX_PAYABLE
REFUND_LIABILITY
```

## Entry
```text
ledger_transaction
ledger_entry
```

Each transaction must balance.

Example:
```text
Customer payment
  DR Customer Receivable
  CR Platform Clearing

Driver earning
  DR Platform Driver Expense
  CR Driver Payable
```

Exact account design depends on accounting requirements.

## Amount
Use:
```text
amount_minor BIGINT
currency CHAR(3)
```

## Immutable
Entries cannot be edited or deleted through ordinary APIs.

Corrections use reversal/adjustment transactions.

## Wallet
A wallet is an account-like balance, not an independent source of truth.

## Reconciliation
Provider settlement data must be reconciled against internal transactions.

## Definition of Done
- ledger entries balance
- historical entries are immutable
- corrections use new transactions
- wallet balances can be reconciled
