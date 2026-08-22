# 112 — Fraud & Risk Engine

## Objective
Detect suspicious activity across payments, rides, deliveries and accounts.

## Signals
Potential signals include:
- repeated failed payments
- excessive account creation
- abnormal cancellation patterns
- unusual location behavior
- device/account relationships
- impossible travel patterns
- repeated promotions
- chargebacks
- suspicious delivery behavior

## Risk Model
```text
Event
 ↓
Features
 ↓
Rules / Scoring
 ↓
Risk Level
 ↓
Action
```

## Risk Levels
```text
LOW
MEDIUM
HIGH
CRITICAL
```

## Actions
- allow
- step-up verification
- manual review
- limit feature
- temporarily hold
- suspend

Automated actions must be reversible and auditable.

## Definition of Done
Risk decisions can be explained through recorded signals and rules.
