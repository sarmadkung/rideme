# 36 — Job Status, Transition & Cancellation Rules

## Objective
Make lifecycle behavior deterministic under retries, disconnects and concurrent commands.

## Transition Matrix
```text
DRAFT -> QUOTED
QUOTED -> REQUESTED
REQUESTED -> SEARCHING
SEARCHING -> ASSIGNED
ASSIGNED -> ACCEPTED
ACCEPTED -> ARRIVING_PICKUP
ARRIVING_PICKUP -> AT_PICKUP
AT_PICKUP -> IN_PROGRESS
IN_PROGRESS -> ARRIVING_DROPOFF
ARRIVING_DROPOFF -> COMPLETED
```

Possible terminal transitions:
```text
SEARCHING -> EXPIRED
SEARCHING -> CANCELLED
ASSIGNED -> CANCELLED
ACCEPTED -> CANCELLED
IN_PROGRESS -> FAILED
```

## Offer Timeout
```text
OFFERED
 -> timeout
 -> release driver reservation
 -> next candidate
```
The job remains active while dispatch continues.

## Reassignment
Driver rejection, timeout, cancellation or loss of eligibility releases the assignment and returns the job to `SEARCHING`.

Never silently overwrite assignment history.

## Cancellation
Example policy:
```text
Before assignment -> normally free
After assignment  -> possible fee
After arrival     -> higher fee
After start       -> normal cancellation not permitted
```
Actual rules are configurable.

## Concurrency
Concurrent accept/start/complete/cancel commands must be serialized using database locking or optimistic versioning.

## Idempotency
Repeated commands must return the authoritative result without corrupting state.

## Audit
Record:
```text
job_id
from_status
to_status
actor_type
actor_id
reason
created_at
metadata
```

## Definition of Done
- transition matrix enforced
- concurrent accepts safe
- retries safe
- assignment timeout works
- reassignment works
- cancellation rules are state-aware
- transitions are auditable
