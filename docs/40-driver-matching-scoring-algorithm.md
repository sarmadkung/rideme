# 40 — Driver Matching & Scoring Algorithm

## Objective
Rank eligible candidates according to service quality, ETA, economics and reliability.

## Initial Score
Use a weighted score rather than a single nearest-driver rule.

Conceptually:
```text
score =
  ETA_WEIGHT * normalized_eta
+ DISTANCE_WEIGHT * normalized_distance
+ RELIABILITY_WEIGHT * reliability
+ ACCEPTANCE_WEIGHT * acceptance_quality
+ IDLE_TIME_WEIGHT * idle_time
+ SERVICE_MATCH_WEIGHT * capability_match
+ FAIRNESS_WEIGHT * supply_fairness
```

Lower-is-better metrics should be normalized before combination.

## Important Signals

### ETA
Prefer estimated arrival time over straight-line distance where routing data is available.

### Distance
Useful as a cheap fallback and early candidate filter.

### Reliability
Use behavior over a rolling window:
- cancellation
- no-show
- acceptance
- completion

Do not permanently punish a driver for isolated events.

### Idle Time
Prevent the same high-performing drivers from receiving every job.

### Capability Match
Exact match should receive a strong preference.

### Fairness
Protect supply health and reduce starvation.

## Special Cases

### Ride
Prioritize ETA and vehicle class.

### Grocery
Consider merchant pickup readiness and vehicle capacity.

### Cargo
Consider capacity, dimensions, helpers and loading requirements.

### Scheduled
Optimize for future availability and reliability rather than only current distance.

## Anti-Gaming
Do not expose the complete scoring formula to drivers.

Monitor:
- suspicious location behavior
- repeated app reconnect patterns
- impossible movement
- coordinated cancellations

## Versioning
Store:
```text
dispatch_strategy_version
score_version
```

Every assignment should be explainable retrospectively.

## Definition of Done
The system can rank candidates and explain the major factors behind an assignment for support/debugging.
