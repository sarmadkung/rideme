# 61 — Payment Testing Strategy

## Objective
Test financial correctness more aggressively than normal CRUD.

## Unit Tests
Test:
- pricing to payment amount
- commission
- earnings
- refunds
- ledger balancing
- currency conversion rules if introduced

## Integration Tests
Use a provider sandbox/mock for:
- authorize
- capture
- refund
- webhook
- payout
- failure

## Idempotency Tests
Repeat:
```text
payment creation
capture
refund
payout
webhook
completion
```

Expected: one financial effect.

## Concurrency Tests
Run parallel:
- capture
- refund
- payout
- earnings creation

## Reconciliation Tests
Inject mismatches and ensure they become exceptions.

## Property Tests
Useful invariant:
```text
sum(debits) == sum(credits)
```

for every ledger transaction.

## Definition of Done
No money-moving path is considered production-ready without idempotency, concurrency and reconciliation tests.
