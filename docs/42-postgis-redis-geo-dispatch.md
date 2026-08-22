# 42 — Geographic Dispatch with PostGIS & Redis

## Objective
Use each geographic technology for the problem it solves best.

## Redis
Use for:
- current driver location
- current online state
- nearby candidate lookup
- short-lived dispatch state

## PostGIS
Use for:
- zones
- service boundaries
- geofencing
- durable geographic records
- historical queries
- complex spatial analysis

## Redis GEO Model
Conceptually:
```text
geo:drivers:available:{capability}
```

Store driver coordinates and retrieve nearby IDs.

Do not treat Redis as the permanent source of truth.

## PostGIS Examples
Useful queries include:
- point inside service zone
- distance from depot
- route corridor analysis
- zone-based pricing
- historical driver density

## Coordinate Rules
Use WGS84 coordinates.

Validate:
```text
latitude -90..90
longitude -180..180
```

## Geofencing
Arrival detection should account for GPS error.

Use configurable radii and hysteresis rather than a single exact coordinate comparison.

## Redis Failure
If Redis is unavailable:
- do not silently dispatch using stale state
- degrade safely
- recover/rebuild operational state from authoritative sources where possible

## Definition of Done
Geo candidate lookup is fast, spatial rules are accurate, and Redis failure cannot create unsafe assignments.
