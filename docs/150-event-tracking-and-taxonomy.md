# 150 — Event Tracking & Taxonomy

## Objective
Define a consistent event vocabulary across web, mobile and backend services.

## Event
```text
event_id
event_name
actor_id
anonymous_id where applicable
timestamp
source
session_id
correlation_id
properties
```

## Naming
Use predictable names:
```text
ride.requested
ride.booked
ride.cancelled
delivery.created
delivery.assigned
delivery.completed
payment.succeeded
```

## Rules
- event names are versioned
- properties have documented meaning
- timestamps are UTC
- sensitive information is excluded
- duplicate events are detectable

## Definition of Done
Product and backend teams use the same event taxonomy.
