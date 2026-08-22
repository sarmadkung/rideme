# 98 — Driver Location Tracking

## Objective
Provide efficient live driver tracking for dispatch, customer ETA and operations.

## Pipeline
```text
Native GPS
 ↓
React Native Location Layer
 ↓
Transport
 ↓
Realtime Gateway
 ↓
Validation
 ↓
Redis Current State
 ↓
Dispatch / Tracking
```

## Data
```text
driver_id
latitude
longitude
accuracy
speed
heading
timestamp
```

## Frequency
Use adaptive tracking:
- online/idle
- heading to pickup
- active trip
- background/inactive

Avoid maximum GPS frequency continuously.

## Validation
Reject or flag:
- invalid coordinates
- stale timestamps
- impossible speed
- unrealistic jumps

## Current vs Historical
Redis stores current operational state.

Historical location should use a separate durable/batched pipeline.

## Definition of Done
Location is fresh enough for dispatch while controlling battery, network and storage costs.
