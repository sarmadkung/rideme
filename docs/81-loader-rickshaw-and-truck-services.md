# 81 — Loader, Rickshaw & Truck Services

## Objective
Support multiple commercial vehicle classes under one driver/vehicle platform.

## Vehicle Classes
```text
MOTORCYCLE
CAR
RICKSHAW
LOADER_RICKSHAW
VAN
PICKUP
TRUCK
```

## Service Types
Examples:
- passenger ride
- grocery delivery
- parcel delivery
- local cargo
- moving assistance
- scheduled transport
- business delivery

## Registration
A driver can register multiple vehicles where allowed.

Each vehicle has:
- registration details
- type
- capacity
- verification status
- service capabilities
- documents

## Service Eligibility
A vehicle may be active for one service and inactive for another.

Example:
```text
Truck → Cargo = enabled
Truck → Ride = disabled
```

## Definition of Done
Vehicle type and service capability are independent concepts and dispatch uses both.
