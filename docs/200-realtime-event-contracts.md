# 200 — Realtime Event Contracts

## Objective
Define realtime events for driver, customer, merchant, and admin clients.

## Events

```text
driver.location.updated
driver.availability.changed
booking.created
booking.status.changed
job.created
job.offered
job.assigned
job.status.changed
payment.status.changed
order.status.changed
message.created
```

## Event envelope

```json
{
  "id": "event-id",
  "type": "job.status.changed",
  "version": 1,
  "occurredAt": "timestamp",
  "entityId": "id",
  "correlationId": "id",
  "payload": {}
}
```

## Rules
- Events are versioned.
- Consumers must be idempotent.
- Do not send sensitive data unnecessarily.
- Reconnect requires state reconciliation.
- Event ordering must not be assumed unless explicitly guaranteed.

## Agent tasks
- Implement event publisher.
- Implement subscriptions.
- Implement reconnect/resync.
- Add event contract tests.

## Acceptance criteria
Critical realtime workflows remain correct across disconnects and duplicate events.
