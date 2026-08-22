# 97 — Geofencing & Service Zones

## Objective
Define geographic business boundaries.

## Zone Types
```text
SERVICE_AREA
DELIVERY_ZONE
PICKUP_ZONE
SURGE_ZONE
RESTRICTED_ZONE
WAREHOUSE_ZONE
MERCHANT_ZONE
```

## Geometry
Use PostGIS polygons/multipolygons.

## Point-in-Zone
Examples:
- is merchant inside service area?
- is destination deliverable?
- did driver enter pickup geofence?
- is surge active?

## Geofence Events
```text
ENTER
EXIT
DWELL
```

## GPS Tolerance
GPS is noisy.

Use configurable radius/tolerance and avoid triggering repeated enter/exit events around a boundary.

## Definition of Done
All geographic business rules use centralized zone definitions rather than hard-coded coordinates.
