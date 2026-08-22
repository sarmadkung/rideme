# 115 — Safety Events & Audit Log

## Objective
Create immutable-style records for important trust and safety decisions.

## Event Examples
```text
identity.verified
vehicle.approved
sos.triggered
incident.created
risk.flagged
account.suspended
document.expired
```

## Event Structure
```text
event_id
event_type
actor
subject
timestamp
source
metadata
correlation_id
```

## Audit Requirements
Important changes must identify:
- who/what caused them
- what changed
- why
- when

## Retention
Retention should be policy-based and appropriate to legal/privacy requirements.

## Definition of Done
Critical safety and enforcement actions can be reconstructed after the fact.
