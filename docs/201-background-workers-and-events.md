# 201 — Background Workers & Events

## Objective
Implement asynchronous processing for tasks that should not block user requests.

## Worker candidates
- notifications
- payment reconciliation
- analytics ingestion
- document processing
- cleanup
- settlement
- retry handling
- event fan-out

## Queue requirements
- durable messages
- retry policy
- dead-letter handling
- idempotency
- visibility/lease timeout
- monitoring

## Rules
Never assume a message is delivered exactly once.

Consumers must safely process duplicates.

## Agent tasks
Implement worker framework, queue abstraction, retry policy, dead-letter handling, and metrics.

## Acceptance criteria
Failed jobs retry safely and permanently failing jobs become visible for operational intervention.
