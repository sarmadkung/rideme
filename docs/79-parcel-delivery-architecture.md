# 79 — Parcel Delivery Architecture

## Objective
Support point-to-point and scheduled parcel delivery using the same core job and dispatch platform.

## Delivery Model
```text
Customer
  ↓
Delivery Order
  ↓
Pickup
  ↓
Dispatch
  ↓
Driver + Vehicle
  ↓
Delivery
  ↓
Proof of Delivery
```

## Supported Modes
- motorcycle parcel
- car parcel
- rickshaw
- loader/rickshaw
- van
- truck
- future specialized vehicles

## Order vs Job
Keep:
```text
Parcel Order
≠
Delivery Job
```

An order describes the customer's commercial request. The job describes the operational movement.

## Definition of Done
Parcel delivery can use the existing booking, quote, dispatch, realtime and payment systems without duplicating them.
