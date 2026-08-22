# 95 — Routing Provider Abstraction

## Objective
Prevent routing-provider lock-in and make routing replaceable.

## Interface
Conceptually:
```text
route(origin, destination, options)
routeMatrix(points, options)
estimateETA(origin, destination, options)
```

## Provider Response
Normalize to:
```text
distance_meters
duration_seconds
geometry
legs
traffic_duration
provider
provider_version
```

## Routing Modes
Support:
- driving
- motorcycle where available
- truck where supported

Do not assume a car route is always valid for a truck.

## Fallback
A provider failure may use a configured fallback provider.

Do not silently mix incompatible routing assumptions.

## Caching
Cache appropriate repeated requests, but avoid stale ETA for active trips.

## Definition of Done
Business logic receives normalized routes regardless of provider.
