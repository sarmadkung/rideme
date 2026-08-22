# 89 — Business Bulk & Recurring Delivery

## Objective
Support businesses that generate many delivery jobs.

## Bulk Order
Accept multiple delivery instructions in one request.

Example:
```text
Order Batch
 ├── Stop 1
 ├── Stop 2
 ├── Stop 3
 └── Stop N
```

## Recurring
Future support:
```text
daily
weekly
custom schedule
```

Generate individual operational jobs from a recurring plan rather than creating one permanently active job.

## API
Bulk APIs require:
- idempotency
- validation
- partial failure reporting

## Pricing
Support batch pricing where commercially required.

## Operations
Provide bulk progress:
```text
created
assigned
picked_up
delivered
failed
```

## Definition of Done
Business customers can create many deliveries without manually creating each job.
