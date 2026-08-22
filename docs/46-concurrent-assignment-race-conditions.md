# 46 — Concurrent Assignment & Race Conditions

## Objective
Make dispatch correct under concurrency.

## Main Race
Two workers may attempt:
```text
Job J -> Driver D
```
at the same time.

Only one may succeed.

## Protection
Use a database transaction with:
- row lock
- unique constraints
- reservation state
- version checks

## Suggested Invariants
At most one active assignment per job.

At most one active job/reservation may consume a driver's dispatch capacity according to the driver's state.

## Idempotency
Commands carry an idempotency key where retries can happen.

## Worker Duplicates
NATS delivery may be repeated.

Consumers must be idempotent.

Use:
```text
event_id
consumer
processed_at
```
or equivalent durable deduplication.

## Transaction Boundaries
Do not perform long network calls inside database transactions.

Prefer:
```text
short DB transaction
 -> commit state
 -> publish/trigger async work
```

Use an outbox pattern where atomic DB + event publication is required.

## Testing
Write concurrency tests that start many assignment attempts simultaneously.

Expected:
```text
1 winner
N losers
0 corrupted jobs
0 double assignments
```

## Definition of Done
Race-condition tests pass repeatedly under parallel execution.
