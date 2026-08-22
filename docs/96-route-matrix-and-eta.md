# 96 — Route Matrix & ETA Engine

## Objective
Efficiently estimate travel time for dispatch, pricing and customer tracking.

## Matrix
For dispatch:
```text
Drivers × Pickup
```

For multi-stop:
```text
Stops × Stops
```

## ETA Inputs
- origin
- destination
- routing mode
- traffic
- timestamp
- vehicle type
- route constraints

## ETA Usage
Different consumers need different freshness:
```text
Dispatch → high freshness
Customer → moderate freshness
Analytics → historical
Pricing → quote-time estimate
```

## Fallback
If live routing fails:
- use cached estimate
- use historical model
- degrade clearly

Never present a fallback as exact.

## Definition of Done
ETA is centralized, measurable and reusable by dispatch, tracking and pricing.
