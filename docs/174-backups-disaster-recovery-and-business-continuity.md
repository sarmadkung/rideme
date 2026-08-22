# 174 — Backups, Disaster Recovery & Business Continuity

## Objective
Recover the platform from infrastructure or data failures.

## Targets
Define:
```text
RPO — acceptable data loss
RTO — acceptable recovery time
```

## Backups
Cover:
- PostgreSQL
- object storage
- critical configuration
- required operational data

## Recovery
Test:
- database restore
- service redeployment
- secret recovery
- infrastructure recreation

## Disaster Scenarios
- database failure
- region/service outage
- accidental deletion
- compromised credentials
- bad deployment

## Definition of Done
Backups are not considered sufficient until restoration has been tested.
