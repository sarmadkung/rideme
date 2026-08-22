# 101 — Map Performance & Caching

## Objective
Keep maps responsive on low-end devices and unstable networks.

## Mobile
Optimize:
- marker count
- update frequency
- map rerenders
- route polyline updates
- image assets
- memory

## Driver Markers
Do not redraw the entire map for every location event.

Update only necessary marker state.

## Web Dashboard
Cluster large driver populations.

Use:
```text
viewport filtering
marker clustering
server-side aggregation
```

## Caching
Cache:
- geocoding results
- static place metadata
- appropriate route results

Avoid caching sensitive live-location data longer than necessary.

## Definition of Done
Map interactions remain responsive under realistic driver/order volumes.
