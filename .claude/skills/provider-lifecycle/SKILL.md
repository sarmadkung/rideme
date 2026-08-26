---
name: provider-lifecycle
description: Driver/provider onboarding, document verification, vehicle registration, availability, shifts, and enforcement. Use for provider registration, verification pipelines, availability toggling, or anything gating a driver's right to accept work.
---

# Purpose

Control who may perform work on the platform, and keep that control server-side.

# When to Use

Provider registration, identity and document verification, vehicle registration, availability, suspension and enforcement.

# States (`docs/16`)

```text
Driver:  OFFLINE → AVAILABLE → OFFERED → ACCEPTED → ON_TRIP → AVAILABLE
         plus PAUSED, SUSPENDED, BLOCKED

Vehicle: PENDING_VERIFICATION → VERIFIED → (SUSPENDED | EXPIRED)
```

# Rules

- **A driver cannot accept jobs with** expired required documents, no verified active vehicle, or a stale location (`docs/16`). Enforced server-side at accept time — a client-side check is a convenience, never the gate.
- **Document expiry is time-based and must be re-evaluated**, not stamped once at verification. A licence valid at onboarding expires later; something must notice.
- **Verification is a pipeline with states**, not a boolean (`docs/384`). Submitted → under review → approved/rejected, with a reason.
- Identity documents are sensitive: restricted access, controlled retention, never in logs or analytics (`docs/108`, `docs/316`).
- Suspension and blocking are enforcement actions with an audit trail (`docs/113`). Who acted, when, why.
- A driver may hold multiple vehicles via `driver_vehicles` with a primary flag (`docs/13`) — do not assume one driver, one vehicle.
- Availability lives in Redis for dispatch speed; the authoritative driver record stays in Postgres.

# Workflow

1. Registration captures identity and creates the provider record in an unverified state.
2. Document submission enters the verification pipeline with explicit states.
3. Vehicle registration and verification run in parallel; capabilities are declared here (`vehicle-service-eligibility`).
4. Availability toggling updates Redis and emits `driver.online` / `driver.offline`.
5. Every gate is re-checked at accept time.

# Verification

Level 5 — this gates who may drive, carry passengers, and be paid.

Required: expired document blocks acceptance, unverified vehicle blocks acceptance, suspended driver cannot go available, document expiring mid-shift, unauthorized access to another provider's documents, availability race between toggle and offer.

# Blocking Conditions

- Required document set per vehicle type or service undocumented → `BLOCKED_TASKS.md`; this is regulatory.
- Document retention period unspecified → privacy decision; ask.
- Suspension criteria and appeal path undefined → do not invent enforcement policy.

# Relevant Documentation

`docs/16-driver-vehicle-state-machine.md` · `docs/29-driver-onboarding-verification.md` · `docs/31-driver-availability-location.md` · `docs/108-driver-identity-and-vehicle-verification.md` · `docs/210-driver-provider-onboarding.md` · `docs/211-driver-verification.md` · `docs/212-driver-availability.md` · `docs/384-document-verification-pipeline.md` · `docs/458-driver-shift-management.md` · `docs/545`–`547` (provider flows)
