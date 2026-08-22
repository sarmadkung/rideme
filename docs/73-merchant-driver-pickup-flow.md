# 73 — Merchant Driver Pickup Flow

## Flow
```text
Order Confirmed
 → Preparing
 → Ready / Predicted Ready
 → Dispatch
 → Driver Arrives
 → Pickup Verification
 → Picked Up
 → Delivery
```

## Pickup Verification
Possible mechanisms:
- order code
- QR code
- merchant confirmation
- driver confirmation

Use stronger verification for high-value orders.

Record driver, vehicle, store, timestamp and verification method.

Handle:
- order not ready
- wrong order
- missing items
- merchant closed
- driver unable to pick up

## Definition of Done
The platform can prove which driver collected which order and when.
