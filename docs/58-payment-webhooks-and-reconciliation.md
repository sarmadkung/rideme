# 58 — Payment Webhooks & Reconciliation

## Objective
Ensure internal financial state eventually matches payment providers.

## Webhook Flow
```text
Provider
 ↓
Webhook Gateway
 ↓
Signature Validation
 ↓
Event Deduplication
 ↓
Payment Handler
 ↓
Ledger / Payment State
```

## Webhook Rules
- verify signatures
- store raw provider event reference where permitted
- deduplicate by event ID
- do not trust client-side payment success
- process asynchronously where appropriate

## Reconciliation Job
Periodically compare:
```text
provider transactions
vs
internal payment transactions
```

Find:
- missing transactions
- duplicate captures
- amount mismatch
- unexpected refunds
- settlement differences

## Exceptions
Create reconciliation cases rather than silently fixing mismatches.

## Definition of Done
Provider state and internal state can be compared and mismatches are surfaced automatically.
