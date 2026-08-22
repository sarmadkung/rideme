# 130 — Support Ticketing & Case Management

## Objective
Turn customer, driver and merchant problems into trackable cases.

## Case States
```text
OPEN
ASSIGNED
IN_PROGRESS
WAITING_CUSTOMER
WAITING_INTERNAL
RESOLVED
CLOSED
```

## Case Data
```text
case_id
requester
category
priority
related_order/job/trip
assignee
created_at
updated_at
resolution
```

## Categories
- payment
- delivery
- ride
- merchant
- driver
- account
- safety
- technical

## SLA
Track response and resolution deadlines by priority.

## Definition of Done
Support can manage a problem from creation through documented resolution.
