# 197 — Domain State Machines

## Objective
Make lifecycle transitions explicit and enforceable.

## Booking

```text
DRAFT
 → QUOTED
 → CONFIRMED
 → ASSIGNED
 → IN_PROGRESS
 → COMPLETED
```

Alternative terminal states:
```text
CANCELLED
EXPIRED
FAILED
```

## Job

```text
CREATED
 → OFFERED
 → ACCEPTED
 → ARRIVING
 → PICKED_UP
 → IN_TRANSIT
 → COMPLETED
```

## Payment

```text
CREATED
 → REQUIRES_ACTION
 → AUTHORIZED
 → CAPTURED
```

Failure/terminal states:
```text
FAILED
CANCELLED
REFUNDED
```

## Rules
- Invalid transitions must fail.
- State changes should be auditable.
- Repeated transition requests must be idempotent.
- State machines must be tested independently.

## Agent tasks
Implement explicit transition services and tests.

## Acceptance criteria
No critical workflow can reach an invalid state through a public API.
