# 60 — Payment APIs & Admin Operations

## Customer APIs
```text
POST /payments/intents
GET  /payments/{id}
POST /payments/{id}/confirm
GET  /payments/methods
```

## Driver APIs
```text
GET  /driver/earnings
GET  /driver/earnings/{job_id}
GET  /driver/payouts
POST /driver/payouts
```

## Merchant APIs
```text
GET /merchant/settlements
GET /merchant/settlements/{id}
```

## Admin APIs
```text
GET  /admin/payments
GET  /admin/refunds
POST /admin/refunds
GET  /admin/reconciliation
GET  /admin/payouts
POST /admin/payouts/{id}/retry
```

Admin endpoints require strict role/permission checks.

## Operational Dashboard
Show:
- payment failures
- pending captures
- refunds
- payout failures
- reconciliation exceptions
- COD mismatches

## Definition of Done
Operational staff can investigate financial issues without direct database modification.
