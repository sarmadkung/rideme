# 103 — Location Events & Realtime

## Objective
Turn validated location changes into efficient realtime updates.

## Event
```json
{
  "event_id": "...",
  "type": "driver.location.updated",
  "driver_id": "...",
  "timestamp": "...",
  "latitude": 0,
  "longitude": 0,
  "heading": 0
}
```

## Consumers
- dispatch
- customer tracking
- driver app
- operations dashboard
- ETA service

## Coalescing
Location is high frequency.

Consumers may receive:
- latest location
- throttled stream
- derived ETA
rather than every raw GPS point.

## Ordering
Do not assume network delivery order.

Use timestamps/sequence information and reject older state where appropriate.

## Definition of Done
Realtime location remains scalable and consumers can recover authoritative state after reconnect.
