# 70 — Grocery Order Lifecycle

## States
```text
CART
PLACED
PAYMENT_PENDING
CONFIRMED
PREPARING
READY_FOR_PICKUP
PICKED_UP
DELIVERING
DELIVERED
CANCELLED
FAILED
```

## Flow
```text
Cart → Place → Payment → Merchant Confirmation → Preparing → Ready → Pickup → Delivery → Delivered
```

Merchant rejection may occur before preparation with a reason such as unavailable items or store closure.

Customer cancellation rules depend on order state.

When appropriate:
```text
READY_FOR_PICKUP → create/activate delivery job
```

Order and delivery state remain separate and communicate through explicit events.

## Definition of Done
A grocery order can move from checkout through merchant fulfillment to completed delivery.
