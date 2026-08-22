# 49 — NATS Dispatch Events

## Objective
Use NATS for asynchronous domain and operational events without making the message bus the source of truth.

## Event Examples
```text
job.created
job.searching
dispatch.started
dispatch.candidate_selected
driver.offer_created
driver.offer_expired
driver.offer_rejected
assignment.created
assignment.released
job.reassigned
driver.location.updated
```

## Subject Convention
Use versioned subjects such as:
```text
logistics.job.created.v1
logistics.dispatch.started.v1
logistics.assignment.created.v1
```

Avoid uncontrolled subject naming.

## Event Envelope
```text
event_id
event_type
version
occurred_at
producer
correlation_id
causation_id
payload
```

## Consumers
Examples:
- notification service
- realtime gateway
- analytics pipeline
- dispatch worker
- audit consumer

## At-Least-Once Delivery
Assume duplicate delivery.

Consumers must be idempotent.

## Ordering
Do not assume global ordering.

If ordering matters, partition/serialize by resource:
```text
job_id
driver_id
```

## Outbox
For critical events:
```text
DB transaction
 -> write domain change
 -> write outbox event
 -> publisher sends event
```

This prevents DB success + event loss.

## Definition of Done
Critical dispatch events are versioned, observable, retryable and idempotently consumed.
