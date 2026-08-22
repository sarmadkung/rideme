# 31 — Driver Availability & Location

## Objective
Implement the operational state required before dispatch can safely offer jobs.

## Availability States
```text
OFFLINE
AVAILABLE
OFFERED
RESERVED
ON_JOB
PAUSED
```

Administrative restrictions:
```text
SUSPENDED
BLOCKED
```

## Go Online Preconditions
Backend checks:
- driver approved
- active vehicle verified
- mandatory documents valid
- effective capabilities exist
- latest location acceptable
- driver not suspended/blocked

## APIs
```text
POST /driver/availability/online
POST /driver/availability/offline
POST /driver/availability/pause
GET  /driver/availability
```

## Location Update
Realtime transport is preferred for active tracking.

Payload:
```json
{
  "latitude": 31.5204,
  "longitude": 74.3587,
  "accuracy_m": 8.5,
  "heading": 120,
  "speed_mps": 7.2,
  "recorded_at": "2026-08-21T12:00:00Z"
}
```

## Validation
Reject or downgrade suspicious points:
- impossible coordinates
- very poor accuracy
- timestamp too old/future
- impossible movement speed
- malformed sequence

Fraud signals should not automatically equal guilt.

## Redis Current State
Conceptually:
```text
driver:{id}:state
driver:{id}:location
geo:available:{zone/capability}
```

Exact key design should be versioned and documented in implementation.

## Location Freshness
Dispatch must use configurable thresholds.

Example:
```text
fresh       < 15 sec
degraded    15–45 sec
stale       > 45 sec
```

These values must be tuned from field data rather than treated as permanent constants.

## Tracking Frequency
Adaptive strategy:
```text
OFFLINE        -> none
AVAILABLE      -> low/moderate
OFFERED        -> moderate
ARRIVING       -> higher
ON_JOB         -> higher
background     -> platform-aware
```

Prefer distance + time thresholds.

## Reconnect
Driver app must:
- detect disconnect
- reconnect with backoff
- refresh authoritative state
- avoid replaying invalid stale commands
- resume location tracking safely

## Definition of Done
- verified driver can go online
- invalid driver cannot go online
- current location appears in Redis
- stale location prevents unsafe dispatch
- offline removes driver from eligible supply
- reconnect restores authoritative state
