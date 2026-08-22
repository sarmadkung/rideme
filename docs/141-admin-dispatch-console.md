# 141 — Admin Dispatch Console

## Objective
Give dispatchers real-time control over active supply and demand.

## Main View
```text
Map
 ├── Drivers
 ├── Pickups
 ├── Active Jobs
 └── Exceptions

Side Panel
 ├── Unassigned
 ├── At Risk
 └── Active
```

## Dispatcher Actions
- assign
- reassign
- cancel eligible job
- contact participants
- view route
- escalate

## Safety
Manual reassignment must respect:
- vehicle capability
- service eligibility
- driver state
- geographic constraints

## Realtime
Use WebSocket updates with API refresh/recovery after reconnect.

## Definition of Done
A dispatcher can actively manage operational demand in real time.
