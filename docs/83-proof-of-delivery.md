# 83 — Proof of Delivery

## Objective
Provide reliable evidence that a parcel/order reached the intended recipient.

## Methods
Support configurable methods:
- recipient OTP
- signature
- photo
- recipient confirmation
- QR/code scan
- merchant confirmation

## Delivery Verification
Do not rely on GPS alone.

A strong completion flow can be:
```text
Driver Arrives
 ↓
Location Check
 ↓
Recipient Verification
 ↓
POD Captured
 ↓
Job Completed
```

## Photo
Store secure media references, not unnecessary duplicate binaries in the job database.

## OTP
Generate short-lived verification codes.

Never expose the raw OTP to unauthorized parties.

## Audit
Store:
- method
- timestamp
- actor
- location metadata where appropriate
- media/reference ID
- verification result

## Definition of Done
Every completed delivery has a policy-compliant proof record.
