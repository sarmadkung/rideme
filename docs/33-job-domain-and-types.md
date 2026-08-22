# 33 — Job Domain & Types

## Objective
Implement the core `Job` abstraction shared by rides, parcel delivery, grocery delivery, and cargo.

## Principle
Do not create completely separate order systems for every service. Use one Job model with service-specific requirements.

```text
Job
├── RIDE
├── PARCEL
├── GROCERY
└── CARGO
```

## Core Job
```text
Job
├── id
├── type
├── requester_id
├── merchant_id
├── status
├── scheduled_at
├── pricing_reference
├── assigned_driver_id
├── assigned_vehicle_id
├── notes
├── created_at
└── updated_at
```

## Job Stops
A job supports ordered stops:
```text
Stop 1: PICKUP
Stop 2: DROPOFF
```
Future multi-stop jobs can use multiple pickups/dropoffs.

Each stop contains latitude, longitude, address, contact information, instructions and sequence.

## Job Requirements
Use an extensible requirement model for passenger count, package weight/dimensions, fragile items, temperature sensitivity, vehicle capability, loading assistance, cash collection and proof of delivery.

## Initial Types

### Ride
- passenger_count
- pickup
- destination
- vehicle_class

### Parcel
- package_count
- weight
- dimensions
- pickup
- destination
- proof_required

### Grocery
- merchant
- order_reference
- package_count
- weight
- cash_collection if applicable

### Cargo
- weight
- dimensions
- loading_required
- vehicle_capability
- helpers_required
- pickup
- destination

## Statuses
```text
DRAFT -> QUOTED -> REQUESTED -> SEARCHING -> ASSIGNED -> ACCEPTED
      -> ARRIVING_PICKUP -> AT_PICKUP -> IN_PROGRESS
      -> ARRIVING_DROPOFF -> COMPLETED
```
Terminal states include `CANCELLED`, `FAILED`, and `EXPIRED`.

Only backend commands can transition status.

## Idempotency
Job creation must accept `Idempotency-Key`. Repeated requests must not create duplicate jobs.

## Definition of Done
- all initial job types are supported
- stops and requirements are reusable
- transitions are validated
- cancellation is audited
- duplicate creation is prevented
