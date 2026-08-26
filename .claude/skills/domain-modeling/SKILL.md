---
name: domain-modeling
description: The canonical RideMe entity model and its state machines — Job as the universal work abstraction, vehicle capabilities, driver states. Use before creating any entity, table, or status field, and whenever a service seems to need its own booking or order type (it does not).
---

# Purpose

Hold one canonical model so ride, parcel, grocery, and cargo specialize a shared core instead of forking four parallel domains.

# When to Use

- Creating or changing an entity, status field, or lifecycle.
- Adding a service type.
- Any moment a second "order"-shaped concept appears.

# The Canonical Model (`docs/04`)

```text
User
  ├── Driver  ──> DriverVehicle ──> Vehicle ──> VehicleCapability
  ├── Customer
  └── Merchant

Job
  ├── JobStop[]        ├── PricingQuote     ├── Payment
  ├── JobRequirement   ├── Assignment       └── Proof[]

Assignment ──> Driver + Vehicle
```

**Job is the universal abstraction.** All operational work is a Job. Types: `RIDE`, `PARCEL`, `GROCERY`, `CARGO`, `FREIGHT`. Common fields: requester, pickup, destination, stops, scheduled_at, requirements, pricing, status, assigned driver, assigned vehicle, payment, audit history.

Grocery adds merchant/catalog/cart concepts *around* the Job — it does not replace it.

# State Machines

**Job** (`docs/15`) — backend commands only; clients never set status directly:
```text
DRAFT → QUOTED → REQUESTED → SEARCHING → ASSIGNED → ACCEPTED
      → ARRIVING → AT_PICKUP → IN_PROGRESS → AT_DROPOFF → COMPLETED
```
Terminal: `CANCELLED`, `FAILED`, `EXPIRED`, `DISPUTED`.
Every transition emits `JobStatusChanged` — job id, previous status, new status, actor, timestamp, metadata.

**Driver** (`docs/16`):
```text
OFFLINE → AVAILABLE → OFFERED → ACCEPTED → ON_TRIP → AVAILABLE
```
Also `PAUSED`, `SUSPENDED`, `BLOCKED`.

**Vehicle** (`docs/16`):
```text
PENDING_VERIFICATION → VERIFIED → (SUSPENDED | EXPIRED)
```

# Rules

- Never create a per-service parallel to `Job`. Specialize with type, requirements, and stops.
- Status is changed by backend commands that validate the transition. An invalid transition is rejected, not coerced.
- Vehicles carry **capabilities**, never a hardcoded service (`vehicle-service-eligibility`).
- One authoritative owner per field (`docs/448`).
- Do not invent states. The documented sets above are the sets.

# Workflow

1. Map the new concept onto the canonical model — usually it is a Job field, a JobRequirement, or a Proof.
2. If it genuinely is a new entity, name its owning module and its authoritative fields.
3. Define valid transitions before writing the table.
4. Enforce invariants in the database (FK, unique, check) as well as in code.
5. Emit the documented event on transition.

# Verification

Level 3 minimum — domain change. Level 5 if it touches assignment, payment linkage, or concurrent transition. Test the rejected transitions, not only the happy path.

# Blocking Conditions

- A required state or transition is undocumented → `BLOCKED_TASKS.md`. Do not invent lifecycle rules.
- A change would give two modules authority over one field → stop.

# Relevant Documentation

`docs/04-domain-architecture.md` · `docs/15-job-state-machine.md` · `docs/16-driver-vehicle-state-machine.md` · `docs/13-database-schema.md` · `docs/445`–`449` (Tier C — titles only)
