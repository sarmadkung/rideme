---
name: event-driven-architecture
description: NATS events, queues, and background workers — delivery semantics, idempotent consumers, versioning. Use when publishing or consuming an event, adding a worker or scheduled job, or when async work must survive retries and duplicates.
---

# Purpose

Make asynchronous work correct under the conditions that actually occur: duplicates, retries, out-of-order delivery, and crashes.

# When to Use

- Publishing or consuming a NATS event.
- Adding a background worker or scheduled job.
- Any work moved off the request path.

# Rules

- **NATS is the message bus** (`docs/12`). Events carry notification and payload — **PostgreSQL remains the source of truth**. A consumer that cannot re-derive state from the database is fragile.
- **Assume at-least-once delivery.** Every consumer is idempotent: same message twice produces the same end state. This is not optional, and it is the single most common defect in this class of code.
- **Every message either acks or nacks.** A silently dropped message is a lost job, a lost payout, a lost notification.
- **Version event payloads** (`docs/376`). Add fields; do not repurpose or remove them without a migration path.
- **Do not chain business rules across events** where a transaction would do. Cross-aggregate consistency via events needs an explicit reconciliation story.
- **Failed messages go somewhere visible** — dead-letter or retry queue with alerting, never `catch {}`.
- Workers carry the correlation ID from the originating request.

# Known Event Surface

Realtime/WebSocket events (`docs/18`): `job.assigned`, `job.accepted`, `job.status_changed`, `driver.location`, `driver.online`, `driver.offline`, `quote.updated`, `payment.updated`, `support.updated`.

Domain event: `JobStatusChanged` on every Job transition (`docs/15`).

Location flow (`docs/18`): `Driver → native GPS → client filtering → WebSocket → location gateway → Redis current state → NATS → dispatch/tracking`.

# Workflow

1. Decide it genuinely belongs off the request path. Synchronous is simpler; prefer it when latency allows.
2. Define the subject and the versioned payload.
3. Make the consumer idempotent — natural key or processed-message record.
4. Implement ack/nack on every path, including the error path.
5. Add metrics: processed, failed, retried, lag.
6. Test duplicate delivery and consumer crash mid-processing.

# Verification

Level 3 for a new worker. Level 5 when the worker touches payments, dispatch assignment, or settlement.

Required cases: duplicate message, out-of-order arrival, consumer crash mid-processing, downstream dependency down, poison message.

# Blocking Conditions

- A consumer cannot be made idempotent because the operation is inherently non-repeatable and has no natural key → stop and design a key. Do not ship "probably won't happen twice".
- Ordering is genuinely required but the transport does not guarantee it → record the constraint before building on it.

# Relevant Documentation

`docs/18-realtime-location-architecture.md` · `docs/49-nats-dispatch-events.md` · `docs/376-event-versioning.md` · `docs/385-background-job-framework.md` · `docs/419-queue-topology.md` · `docs/420-queue-message-contracts.md` · `docs/438-worker-idempotency.md`
