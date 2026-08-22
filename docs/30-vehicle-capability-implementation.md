# 30 — Vehicle & Capability Implementation

## Objective
Represent vehicles independently from services so one vehicle can perform every job it is eligible for.

## Initial Vehicle Types
```text
MOTORCYCLE
RICKSHAW
CAR
LOADER_RICKSHAW
SUZUKI_PICKUP
SHEHZORE
MAZDA
TRUCK
```

Vehicle taxonomy must be configuration-friendly because local names/categories can evolve.

## Vehicle Record
```text
Vehicle {
  id
  owner_user_id
  type
  make
  model
  year
  plate_number
  color
  capacity_kg
  dimensions
  verification_status
}
```

## Capabilities
Initial capability examples:
```text
PASSENGER
PARCEL
GROCERY
BUSINESS_DELIVERY
SMALL_CARGO
HOUSE_MOVING
HEAVY_CARGO
INTERCITY
```

## Capability Rules
Capabilities can be derived from:
- vehicle type
- capacity
- verification
- driver license/category
- market configuration
- optional vehicle equipment

Do not trust capabilities submitted by the client.

Backend determines final eligibility.

## Example
```text
Motorcycle
  PASSENGER
  PARCEL
  GROCERY

Suzuki Pickup
  PARCEL
  BUSINESS_DELIVERY
  SMALL_CARGO
  HOUSE_MOVING

Truck
  HEAVY_CARGO
  INTERCITY
```

These are configuration examples, not permanent hard-coded rules.

## Vehicle Verification States
```text
PENDING
UNDER_REVIEW
VERIFIED
REJECTED
SUSPENDED
EXPIRED
```

## Driver-Vehicle Relationship
A driver may register multiple vehicles.

For MVP, a driver selects one active vehicle before going online.

```text
Driver
 -> Active Vehicle
 -> Effective Capabilities
 -> Eligible Jobs
```

## Vehicle Documents
Potential:
- registration
- permit
- insurance where required
- inspection
- vehicle photos

Requirements are market-configurable.

## APIs
```text
POST   /driver/vehicles
GET    /driver/vehicles
GET    /driver/vehicles/{id}
PATCH  /driver/vehicles/{id}
POST   /driver/vehicles/{id}/documents
POST   /driver/vehicles/{id}/activate
GET    /driver/vehicles/{id}/capabilities
```

## Admin
```text
GET  /admin/vehicles
GET  /admin/vehicles/{id}
POST /admin/vehicles/{id}/verify
POST /admin/vehicles/{id}/reject
POST /admin/vehicles/{id}/suspend
```

## Definition of Done
- driver can register vehicles
- documents can be attached
- admin can verify/reject
- driver can select an active verified vehicle
- backend calculates effective capabilities
- ineligible jobs cannot be accepted
