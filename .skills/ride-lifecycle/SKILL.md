---
name: ride-lifecycle
description: The end-to-end RIDE journey — request, quote, dispatch, offer, pickup, trip, completion, payment, rating. Use for any passenger-ride work. This is the platform's first vertical slice and the reference implementation every other service specializes.
---

# Purpose

Deliver one complete working ride, end to end, as the template for delivery, grocery, and cargo.

# When to Use

Any work on passenger rides, and when building a later service that should mirror this shape.

# The Journey

```text
request → quote → confirm → dispatch → offer → accept → navigate
→ arrive → pickup → trip → complete → payment → receipt → rating
```

Mapped onto the canonical Job states (`docs/15`):

```text
DRAFT → QUOTED → REQUESTED → SEARCHING → ASSIGNED → ACCEPTED
      → ARRIVING → AT_PICKUP → IN_PROGRESS → AT_DROPOFF → COMPLETED
```

A ride is `Job` with `type = RIDE` (`docs/04`). It does **not** get its own booking entity.

# Rules

- Every stage is a backend-validated transition emitting `JobStatusChanged`. The client requests; the server decides.
- **Quote before commitment.** Ride pricing (`docs/05`): `base + distance + time + demand + vehicle adjustment`. Where uncertainty is material, present a range (e.g. `Rs 1,800–2,100`) rather than false precision.
- **The client never computes the fare.** It displays what the server returned.
- Booking and acceptance are idempotent. A retried request must not create a second ride or a second charge.
- Cancellation fees follow the documented tiers (`docs/05`): before driver movement → none or low; after meaningful driver travel → compensation; after arrival → higher fee. Do not invent amounts.
- Payment settles against the authoritative ledger (`payment-flow`, `financial-ledger`).
- Ride sharing and scheduled rides are separate documented features (`docs/351`, `docs/352`) — not implicit in the base ride.

# Workflow

1. Confirm prerequisites exist: Job core, pricing/quote, dispatch, location, realtime, payment.
2. Build the thinnest path through every stage before adding refinements.
3. Wire realtime updates per `realtime-architecture` — including reconnect resync.
4. Complete the payment and ledger legs; a ride that ends without a settled financial record is not complete.
5. Prove it end to end before starting another service.

# Verification

Level 4 for individual stages; **Level 5** for dispatch, payment, and cancellation-fee stages.

The slice is done when one automated E2E journey covers request→rating, plus: cancellation at each documented tier, driver decline and reassignment, customer disconnect and resync mid-trip, duplicate booking request, payment failure.

# Blocking Conditions

- Cancellation fee amounts or thresholds not documented → `BLOCKED_TASKS.md`. These are commercial decisions.
- Surge/demand multiplier behaviour unspecified → ask before implementing `demand` in the fare.
- Rating effects on driver reliability undefined → do not invent scoring impact.

# Relevant Documentation

`docs/05-dispatch-pricing.md` · `docs/15-job-state-machine.md` · `docs/533-ride-booking-flow.md` · `docs/534-ride-quote-flow.md` · `docs/535-ride-matching-flow.md` · `docs/536-ride-execution-flow.md` · `docs/111-ratings-reviews-and-quality.md` · `docs/109-trip-safety-and-emergency-sos.md`
