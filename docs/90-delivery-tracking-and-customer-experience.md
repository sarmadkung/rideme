# 90 — Delivery Tracking & Customer Experience

## Objective
Provide customers with useful live delivery visibility.

## Customer Timeline
```text
Order Confirmed
 ↓
Preparing
 ↓
Driver Assigned
 ↓
Driver Arriving
 ↓
Picked Up
 ↓
On the Way
 ↓
Delivered
```

## Map
Show:
- driver location where policy allows
- destination
- ETA
- route/progress

Do not expose unnecessary driver personal information.

## ETA
ETA should be treated as an estimate, not a guarantee.

Update when:
- driver route changes
- traffic changes
- stop sequence changes
- preparation is delayed

## Realtime
Use WebSocket events with REST/API state as the authoritative fallback.

## Definition of Done
Customer can understand where the delivery is, what happens next and whether any delay requires action.
