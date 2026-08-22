# 131 — Support Routing & Operations

## Objective
Route cases to the correct team and provide useful operational context.

## Routing
Examples:
```text
Payment → Finance
Safety → Trust & Safety
Merchant → Merchant Operations
Driver → Driver Operations
Technical → Product/Engineering
```

## Priority
```text
P0 SAFETY
P1 CRITICAL
P2 HIGH
P3 NORMAL
P4 LOW
```

## Agent Context
Show authorized:
- customer
- driver
- merchant
- trip/order/job
- payment status
- recent events
- previous cases

Do not expose unrelated sensitive data.

## Escalation
Cases can escalate based on:
- SLA breach
- severity
- repeated contact
- safety risk

## Definition of Done
Cases are automatically routed where possible and agents receive sufficient context without unnecessary data access.
