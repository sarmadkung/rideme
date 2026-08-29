---
name: delivery-lifecycle
description: The PARCEL journey — booking, quote, dispatch, pickup, delivery, proof, exceptions, and multi-stop. Use for parcel delivery work, proof-of-delivery handling, and delivery failure or return flows.
---

# Purpose

Deliver parcels reliably, including the paths where delivery does not succeed.

# When to Use

Parcel booking, execution, proof of delivery, multi-stop routing, delivery exceptions and returns.

# The Journey

```text
book → quote → dispatch → pickup (proof) → transit → deliver (proof)
→ payment → tracking closed
```

A parcel is `Job` with `type = PARCEL`, using `JobStop[]` for pickup and dropoff (`docs/04`). Multi-stop uses ordered `job_stops` with a sequence (`docs/13`, `docs/82`).

# Rules

- **Proof is evidence, not decoration** (`docs/15`, `docs/83`). Documented mechanisms: pickup photo, pickup confirmation, delivery OTP, proof of delivery. Proof attaches to the Job as `Proof[]` and is captured server-side.
- **Delivery OTP is verified on the server.** A client that decides whether the code matched is a fraud vector.
- **Failure is a first-class path** (`docs/84`). Recipient absent, refused, wrong address, damaged — each needs a defined state and a defined financial consequence. A parcel that cannot be delivered still needs a resolution and a return flow.
- Parcel pricing (`docs/05`): `base + distance + size/weight + urgency`.
- Multi-stop: stops complete in sequence; skipping a stop is an exception, not a silent reorder.
- Eligibility by capability, never vehicle type (`vehicle-service-eligibility`).
- Scheduled and recurring delivery are separate documented features (`docs/85`, `docs/89`).

# Workflow

1. Reuse the ride slice's Job, quote, dispatch, and payment paths — parcel specializes, it does not duplicate.
2. Add `JobStop` handling and stop sequencing.
3. Implement proof capture and server-side verification at pickup and dropoff.
4. Implement the exception and return paths before calling it complete.
5. Wire customer tracking through the existing realtime channel.

# Verification

Level 4 for the normal journey; Level 5 where payment, COD, or proof verification is involved.

Required: successful delivery with proof, OTP mismatch rejected, recipient absent, refused delivery, return flow, multi-stop out-of-order attempt, duplicate proof submission.

# Blocking Conditions

- Financial consequence of a failed delivery (who pays, what is refunded) undocumented → `BLOCKED_TASKS.md`.
- Proof retention period unspecified → photos of addresses and recipients are sensitive; ask.
- Return-leg pricing undefined → do not invent it.

# Relevant Documentation

`docs/79-parcel-delivery-architecture.md` · `docs/82-multi-stop-delivery.md` · `docs/83-proof-of-delivery.md` · `docs/84-delivery-failure-and-return-flow.md` · `docs/86-delivery-pricing-and-distance.md` · `docs/90-delivery-tracking-and-customer-experience.md` · `docs/91-delivery-exceptions-and-operations.md` · `docs/541-parcel-booking-flow-implementation.md` · `docs/542-parcel-execution-flow.md`
