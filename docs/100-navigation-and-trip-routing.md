# 100 — Navigation & Trip Routing

## Objective
Support active driver navigation without coupling the entire application to a map SDK.

## Navigation Flow
```text
Assigned
 ↓
Pickup Route
 ↓
Arrive
 ↓
Pickup
 ↓
Destination Route
 ↓
Delivery
```

## Route Recalculation
Recalculate when:
- driver deviates significantly
- destination changes
- major route change
- ETA becomes invalid

Avoid recalculating on every GPS update.

## Navigation Options
The app may:
1. provide embedded navigation
2. launch external navigation
3. integrate a native navigation SDK

The architecture should support more than one approach.

## React Native
Use shared navigation state and native SDK adapters where required for high-performance navigation.

## Definition of Done
Driver can navigate reliably while routing SDK details remain replaceable.
