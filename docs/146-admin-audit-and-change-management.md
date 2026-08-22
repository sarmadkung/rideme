# 146 — Admin Audit & Change Management

## Objective
Make every sensitive administrative action traceable.

## Audit Event
```text
actor
role
action
resource
resource_id
before
after
reason
timestamp
request_id
```

## Sensitive Actions
Examples:
- suspend account
- change pricing
- issue refund
- change service zone
- modify permissions
- alter payout configuration

## Before/After
Store structured change information where policy permits.

Do not store unnecessary secrets or sensitive raw data.

## Change Approval
High-impact configuration may require two-person approval.

## Definition of Done
Security and operations can reconstruct who changed what, when and why.
