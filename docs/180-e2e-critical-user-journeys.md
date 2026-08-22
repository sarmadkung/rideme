# 180 — E2E Critical User Journeys

## Objective
Validate complete cross-service workflows.

## Ride
```text
Customer
 → Select Ride
 → Quote
 → Book
 → Driver Assigned
 → Pickup
 → Trip
 → Payment
 → Rating
```

## Grocery
```text
Browse
 → Cart
 → Checkout
 → Merchant Accepts
 → Driver Assigned
 → Pickup
 → Delivery
 → POD
```

## Cargo
```text
Create Shipment
 → Quote
 → Vehicle Match
 → Pickup
 → Multi-Stop
 → Delivery
 → POD
 → Settlement
```

## Tools
Use Playwright for web/admin E2E where applicable and an appropriate mobile E2E strategy for React Native.

## Definition of Done
Critical workflows pass in a production-like environment before release.
