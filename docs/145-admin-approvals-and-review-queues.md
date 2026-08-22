# 145 — Admin Approval & Review Queues

## Objective
Centralize workflows requiring human review.

## Queues
```text
Driver Verification
Vehicle Verification
Merchant Onboarding
Document Review
Fraud Review
Refund Approval
Incident Review
Appeals
```

## Review Item
```text
type
subject
priority
status
assigned_to
created_at
due_at
decision
reason
```

## Assignment
Support:
- manual assignment
- team assignment
- automatic routing

## SLA
Track queue age and overdue items.

## Definition of Done
Human-review workflows are visible, assignable and measurable.
