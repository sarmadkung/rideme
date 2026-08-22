# 143 — Admin Service Zones & Rules

## Objective
Allow operations to manage geographic service coverage.

## Zone Configuration
```text
name
geometry
service_types
vehicle_types
status
priority
```

## Rules
Examples:
- delivery available
- ride available
- cargo restriction
- surge
- service fee
- operating hours

## Overlap
Define deterministic priority for overlapping zones.

## Validation
Before activation:
- geometry validity
- supported service
- conflict checks

## Definition of Done
Operations can change geographic business rules without deploying application code.
