# 56 — Merchant Settlement & COD

## Objective
Support grocery/merchant delivery payments and cash-on-delivery without corrupting financial accounting.

## Merchant Flow
```text
Merchant Order
 ↓
Delivery Job
 ↓
Customer Payment / COD
 ↓
Delivery Completion
 ↓
Merchant Receivable
 ↓
Settlement
```

## COD
When driver collects cash:
```text
COD collected
 -> record collection
 -> reconcile expected amount
 -> driver cash liability
 -> merchant/platform settlement
```

Do not treat driver-reported cash as automatically correct.

## Proof
Require appropriate:
- order reference
- amount
- completion timestamp
- delivery proof
- optional OTP

## Reconciliation
Compare:
```text
order amount
expected COD
driver reported amount
settled amount
```

Differences become exceptions.

## Merchant Settlement
Settlement may include:
- order amount
- delivery fees
- platform commission
- refunds
- adjustments

## Definition of Done
COD collections and merchant settlements are independently auditable and reconcilable.
