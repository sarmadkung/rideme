# 126 — Realtime Notifications & Event Bus

## Objective
Connect domain events to notifications and realtime UI updates.

## Example
```text
order.ready
    ↓
Event Bus
 ├── Push
 ├── In-App
 ├── Driver Realtime
 └── Operations
```

## Event Requirements
Events should include:
```text
event_id
event_type
occurred_at
aggregate_id
actor
payload
```

## Reliability
Consumers must be idempotent.

Retries must not send unlimited duplicate notifications.

## Ordering
Some notification streams require ordering by aggregate/job.

## Definition of Done
Domain events can safely drive multiple communication consumers.
