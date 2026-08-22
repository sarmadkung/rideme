# 82 — Multi-Stop Delivery

## Objective
Support jobs with multiple pickups and/or deliveries.

## Example
```text
Warehouse
   ↓
Customer A
   ↓
Customer B
   ↓
Customer C
```

## Stop Model
```text
sequence
type
address
coordinates
contact
instructions
status
arrived_at
completed_at
```

## Stop Types
`PICKUP`, `DELIVERY`, `WAYPOINT`

## State
```text
PENDING
ARRIVING
ARRIVED
IN_PROGRESS
COMPLETED
FAILED
SKIPPED
```

## Ordering
MVP should use a customer-defined sequence.

Future optimization may reorder stops where business rules permit.

## Failure
A failed stop should not automatically invalidate every remaining stop.

The workflow determines whether:
- retry
- skip
- return
- escalate

## Definition of Done
A driver can execute multiple ordered stops with each stop independently auditable.
