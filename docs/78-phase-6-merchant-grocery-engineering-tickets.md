# 78 — Phase 6 Merchant & Grocery Engineering Tickets

## MER-001 — Merchant Domain
Implement merchant and store entities.

## MER-002 — Merchant Onboarding
Registration, verification, approval and audit workflow.

## MER-003 — Store Operations
Locations, operating hours and temporary closures.

## MER-004 — Catalog
Categories, products, variants and price snapshots.

## MER-005 — Inventory
Availability and atomic reservation.

## MER-006 — Grocery Cart
Cart and substitution preferences.

## MER-007 — Checkout
Validate catalog, inventory, delivery area and final pricing.

## MER-008 — Grocery Order
Implement the order state machine.

## MER-009 — Merchant Dashboard
Implement the React order-management dashboard.

## MER-010 — Preparation SLA
Track preparation time and delays.

## MER-011 — Driver Pickup
Implement pickup verification.

## MER-012 — Item Issues
Substitution, removal and partial refund workflow.

## MER-013 — Merchant Settlement
Connect orders to financial settlement.

## MER-014 — Merchant Webhooks
Signed outbound events with retry handling.

## MER-015 — Merchant API
Secure integration endpoints and idempotency.

## MER-016 — Grocery E2E
```text
Customer → Browse → Cart → Checkout → Payment
→ Merchant Accepts → Preparation → Driver Assignment
→ Pickup → Delivery → Completion → Settlement
```

## Phase 6 Exit Criteria
A merchant can onboard, publish products, receive grocery orders, prepare them, hand them to a driver, and reconcile the completed order financially.

Next phase: **Advanced Delivery, Parcel & Cargo Operations**.
