# 185 — Data Consistency & Idempotency

## Objective
Prevent duplicate actions and inconsistent distributed state.

## Idempotent Operations
Examples:
- create order
- payment request
- assignment
- cancellation
- refund
- notification
- POD submission

## Idempotency Key
```text
client/request id
+
operation scope
```

## State Machines
Critical workflows should reject invalid transitions.

Example:
```text
ASSIGNED → PICKED_UP
```
but not:
```text
CANCELLED → PICKED_UP
```
unless an explicit recovery workflow exists.

## Definition of Done
Retries and duplicated messages do not create duplicate financial or operational effects.
