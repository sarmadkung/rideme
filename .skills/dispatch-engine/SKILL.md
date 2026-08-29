---
name: dispatch-engine
description: Driver-job matching — candidate filtering, scoring, offers, reservation, timeout, and the concurrency rules that stop two drivers taking one job. Use for any dispatch, matching, offer, or assignment work. This is a Level 5 area; treat every change as high risk.
---

# Purpose

Assign the best compatible driver-vehicle pair — not merely the nearest — without ever double-assigning a job.

# When to Use

- Matching, scoring, offers, assignment, reassignment, dispatch timeout or retry.
- Any change to `assignments` or driver availability.

# Rules

- **Best compatible, not nearest** (`docs/05`). Proximity is one input among many.
- **Filter before scoring.** Reject a candidate outright when: vehicle capability mismatch · capacity insufficient · driver offline · driver unavailable · required documents expired · vehicle not verified · restricted zone · safety/risk rule triggered (`docs/05`).
- **Stale locations are excluded** (`docs/16`). A driver whose last fix is old is not a candidate.
- **One job, one assignment.** Concurrent acceptance must be resolved by the database — row lock or unique constraint on active assignment. Two drivers must never both receive a confirmed job. Redis locks are an optimization, never the guarantee (`docs/418`).
- **Offers expire.** Timeout, then retry with the next candidate (`docs/44`, `docs/228`). A stuck `SEARCHING` job needs a defined terminal path.
- **Weights are configuration**, not constants in code (`docs/05`).
- Dispatch decisions are server-side and auditable. Log why a candidate was chosen and why others were filtered.

# Scoring Model (`docs/05`)

```text
score =  w1*eta_score + w2*vehicle_fit + w3*driver_reliability
       + w4*route_compatibility + w5*price_fit + w6*customer_preference
       + w7*destination_demand - w8*empty_km - w9*cancellation_risk
```

Route compatibility matters: a driver already travelling Lahore→Gujranwala should be favoured for jobs along that route, to reduce empty kilometres. Return-load search before a long-distance job completes is documented in `docs/05` — implement only when that slice is scheduled.

# Workflow

1. Establish the job's requirements and vehicle eligibility (`vehicle-service-eligibility`).
2. Query candidates via PostGIS proximity plus availability in Redis.
3. Apply hard filters. Log the rejection reason per candidate.
4. Score, rank, offer.
5. Handle accept / decline / timeout, each with a defined next state.
6. Commit the assignment inside a transaction that makes double-assignment impossible.

# Verification

**Always Level 5.** Required: concurrent acceptance by two drivers (the assignment must resolve to exactly one), offer timeout and retry, no eligible candidates, driver goes offline mid-offer, stale-location exclusion, duplicate accept request with the same idempotency key.

Never mark dispatch verified without an actual concurrency test.

# Blocking Conditions

- Scoring weights are unspecified for a new service type → `BLOCKED_TASKS.md`; do not invent weights.
- Correct behaviour when no candidate exists is undefined for a service → ask.
- Fairness or driver-rotation policy is required but undocumented → product decision.

# Relevant Documentation

`docs/05-dispatch-pricing.md` · `docs/38-dispatch-engine-architecture.md` · `docs/39-driver-candidate-discovery.md` · `docs/40-driver-matching-scoring-algorithm.md` · `docs/42-postgis-redis-geo-dispatch.md` · `docs/43-driver-offer-reservation-system.md` · `docs/44-dispatch-timeout-retry-strategy.md` · `docs/45-reassignment-failure-handling.md` · `docs/46-concurrent-assignment-race-conditions.md` · `docs/334-dispatch-testing.md`
