# 140 — Admin Order & Job Operations

## Objective
Provide a unified operational view of rides, deliveries and cargo jobs.

## Search
Search by:
- order ID
- job ID
- customer
- driver
- merchant
- vehicle
- phone/contact reference where authorized

## Job View
Show:
```text
timeline
current state
assignment
route
stops
payment state
exceptions
messages
POD
```

## Actions
Controlled actions may include:
- reassign
- cancel
- reschedule
- retry
- escalate

## Principle
Actions invoke domain workflows and generate audit events.

## Definition of Done
Operations can investigate and resolve abnormal jobs from one interface.
