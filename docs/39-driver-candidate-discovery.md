# 39 — Driver Candidate Discovery

## Objective
Find a sufficiently small set of plausible drivers before expensive scoring.

## Inputs
- pickup location
- job type
- required capabilities
- vehicle requirements
- scheduled time
- service zone
- maximum pickup radius

## Candidate Pipeline
```text
Geo Search
  ↓
Availability Filter
  ↓
Capability Filter
  ↓
Vehicle Filter
  ↓
Freshness Filter
  ↓
Eligibility Filter
  ↓
Scoring
```

## Geo Search
Use Redis GEO for current nearby availability.

PostGIS remains useful for:
- durable zones
- complex geographic rules
- historical analysis
- fallback queries

## Search Radius
Do not use one global radius.

Start with configurable rings:

```text
0–2 km
2–5 km
5–10 km
10–20 km
```

Expand only when supply is insufficient.

## Freshness
Exclude stale location states from normal dispatch.

Example configurable thresholds:
```text
fresh < 15s
degraded 15–45s
stale > 45s
```

## Eligibility
Check:
- driver approved
- vehicle verified
- vehicle active
- required capability
- correct availability state
- no conflicting reservation
- document validity
- service zone restrictions

## Candidate Limit
Do not score thousands of drivers.

Example:
```text
geo candidates: 100
after eligibility: 30
score top candidates: 10
offer: 1 or controlled batch
```

Tune using production metrics.

## Definition of Done
Candidate discovery is fast, deterministic enough for testing, and never returns a driver who is obviously ineligible.
