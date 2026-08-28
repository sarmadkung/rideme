---
name: vehicle-service-eligibility
description: Decides which vehicles and drivers may serve which jobs, via the capability model rather than hardcoded vehicle-to-service mapping. Use when adding a vehicle type or service, filtering dispatch candidates, or validating that a driver may accept a job.
---

# Purpose

Keep one capability-based eligibility rule, so adding a vehicle type or service does not require editing conditionals across the codebase.

# When to Use

- Adding a vehicle type or service type.
- Filtering dispatch candidates.
- Validating a driver's right to accept a specific job.

# The Capability Model (`docs/04`)

A vehicle has **capabilities**, not a hardcoded service:

```text
Motorcycle   passenger=true  parcel=true  grocery=true  heavy_cargo=false
Suzuki       passenger=false parcel=true  grocery=true  small_cargo=true  heavy_cargo=false
```

Stored as `vehicles` plus `vehicle_capabilities` (`docs/13`).

Documented vehicle types (`docs/486`): rickshaw, motorcycle, car, loader, van, pickup, truck — plus future types, which is precisely why the model is capability-based.

# Eligibility Rules

A driver-vehicle pair may serve a job only when **all** hold:

1. Vehicle capability matches the job's service requirement.
2. Vehicle capacity (weight, dimensions) satisfies the job requirement.
3. Vehicle status is `VERIFIED` (`docs/16`).
4. Driver has no expired required document (`docs/16`).
5. Driver has a verified active vehicle.
6. Driver location is not stale.
7. Driver state permits it (`AVAILABLE`, not `SUSPENDED`/`BLOCKED`).
8. No zone or safety restriction applies (`docs/05`).

This is the same filter set dispatch applies before scoring — keep one implementation, called from both dispatch and the accept path. Duplicating it is how the two drift.

# Rules

- **Never hardcode vehicle-type → service.** New types must work by declaring capabilities.
- Capability and capacity are separate checks. A motorcycle may carry parcels but not a 200 kg one.
- Eligibility is evaluated **server-side at accept time**, not only at offer time — state changes between the two.
- Do not invent capability names. Extend the documented set through an ADR.

# Workflow

1. Express the job's requirement as capability + capacity + constraints.
2. Call the shared eligibility check.
3. On rejection, return a reason usable for logging and driver-facing messaging.
4. For a new vehicle type, add the type and its capabilities — no branching logic.

# Verification

Level 3 normally; Level 5 when the change alters dispatch filtering.

Required: each rejection reason fires correctly, boundary capacity, expired document blocks acceptance, unverified vehicle blocks acceptance, eligibility re-checked at accept and not just at offer.

# Blocking Conditions

- A service needs a capability not documented anywhere → ADR plus product confirmation. Do not invent capability semantics.
- Capacity limits for a vehicle type are unspecified → ask; guessing weight limits has safety consequences.

# Relevant Documentation

`docs/04-domain-architecture.md` · `docs/13-database-schema.md` · `docs/16-driver-vehicle-state-machine.md` · `docs/30-vehicle-capability-implementation.md` · `docs/41-vehicle-capability-matching.md` · `docs/80-cargo-and-vehicle-capacity.md` · `docs/214-vehicle-types-and-capabilities.md` · `docs/216-service-eligibility-engine.md` · `docs/485-service-capability-matrix.md` · `docs/486-vehicle-type-catalog.md`
