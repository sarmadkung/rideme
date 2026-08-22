# 84 — Delivery Failure & Return

## Objective
Handle failed delivery attempts without losing the parcel's operational state.

## Failure Reasons
Examples:
- recipient unavailable
- wrong address
- recipient rejected
- damaged package
- access blocked
- merchant issue

## Flow
```text
Delivery Attempt
 ↓
FAILED
 ↓
Business Rule
 ├── Retry
 ├── Reschedule
 ├── Return
 └── Escalate
```

## Return
Return may create:
```text
Return Stop
```

rather than creating an unrelated manual job.

## Retry
Retry limits should be configurable.

## Customer Communication
Use customer-safe messages and avoid exposing internal failure codes.

## Definition of Done
Failed deliveries have deterministic next actions and remain fully traceable.
