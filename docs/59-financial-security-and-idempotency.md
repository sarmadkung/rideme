# 59 — Financial Security & Idempotency

## Objective
Protect every money-moving operation from retries, duplication and unauthorized access.

## Required Controls
- idempotency keys
- authorization
- transaction boundaries
- immutable ledger
- audit logs
- provider webhook verification
- rate limiting
- fraud/risk checks

## Idempotency Record
Conceptually:
```text
key
actor
operation
request_hash
response
status
created_at
expires_at
```

A reused key with a different request must be rejected.

## Race Conditions
Examples:
```text
two payout requests
two captures
two refunds
two completion events
```

Use database constraints and transactions.

## Sensitive Data
Do not store raw card numbers or unnecessary banking credentials.

Use provider tokenization.

## Admin Financial Actions
Require:
- explicit permission
- reason
- audit
- preferably maker/checker approval for high-risk operations

## Definition of Done
Parallel/retry tests cannot create duplicate money movements.
