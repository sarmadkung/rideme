# 92 — Phase 7 Delivery & Cargo Engineering Tickets

## DEL-001 — Parcel Order
Implement parcel-specific order requirements.

## DEL-002 — Cargo Attributes
Add weight, dimensions, volume and handling requirements.

## DEL-003 — Vehicle Capacity
Implement hard capacity matching.

## DEL-004 — Multi-Stop Jobs
Implement ordered pickup/delivery stops.

## DEL-005 — Proof of Delivery
Implement OTP, photo/signature and configurable POD policies.

## DEL-006 — Delivery Failure
Implement retry, reschedule, return and escalation.

## DEL-007 — Scheduled Delivery
Implement future delivery windows and dispatch planning.

## DEL-008 — Cargo Pricing
Add weight, dimension, distance, waiting and stop pricing modifiers.

## DEL-009 — Waiting/Loading
Track loading, unloading and chargeable waiting time.

## DEL-010 — Restricted Cargo
Implement configurable restrictions and manual review.

## DEL-011 — Bulk Delivery
Support business batch creation and progress tracking.

## DEL-012 — Recurring Delivery
Generate future jobs from recurring plans.

## DEL-013 — Delivery Tracking
Implement customer live tracking and ETA.

## DEL-014 — Exception Center
Implement structured operational exceptions.

## DEL-015 — Cargo E2E
```text
Customer/Business
 → Quote
 → Create Parcel
 → Driver Match
 → Pickup
 → Multi/Single Stop
 → POD
 → Completion
 → Payment/Earnings
```

## Phase 7 Exit Criteria
The platform can safely handle motorcycle parcels through large cargo jobs, including capacity matching, scheduled/multi-stop delivery, proof of delivery, failed-delivery recovery and operational exceptions.
