---
name: cargo-lifecycle
description: The CARGO journey — dimensions and weight, vehicle requirements, quoting, loader/truck assignment, loading, transport, delivery proof, and settlement. Use for cargo, loader, rickshaw-freight, and truck work, where capacity and safety constraints are real.
---

# Purpose

Move freight with vehicles that can actually carry it, priced for the work involved.

# When to Use

Cargo requests, capacity matching, loader/truck assignment, loading and waiting time, cargo documents.

# The Journey

```text
request (dimensions, weight, requirements) → quote → provider matching
→ truck/loader assignment → loading → transport → delivery → proof → settlement
```

Cargo is `Job` with `type = CARGO` (`FREIGHT` also exists in `docs/04`). Capacity is the defining constraint.

# Rules

- **Capacity is a hard filter, never a scoring input.** A vehicle that cannot carry the load is ineligible — weight *and* dimensions (`docs/80`, `vehicle-service-eligibility`).
- Cargo pricing (`docs/05`): `base + distance + vehicle + capacity + loading + waiting + schedule`. Loading and waiting time are billable components (`docs/87`), so they must be recorded as events with timestamps, not estimated after the fact.
- **Safety and cargo restrictions apply** (`docs/88`). Restricted goods and restricted zones are filters, and they exist for legal reasons — do not treat them as advisory.
- Loader services (`docs/81`) may involve labour as well as transport; that is a documented service characteristic, not an implicit assumption.
- Long-distance cargo interacts with route compatibility and return loads in dispatch (`docs/05`) — implement when that slice is scheduled, not speculatively.
- Cargo documents and compliance (`docs/275`) attach to the Job.

# Workflow

1. Capture dimensions, weight, and requirements as structured `JobRequirement` data — not free text, since dispatch must filter on it.
2. Quote using the documented cargo formula.
3. Filter candidates on capability plus capacity plus restrictions.
4. Record loading start/end and waiting time as timestamped events.
5. Capture delivery proof and settle.

# Verification

Level 4 for the journey; Level 5 for quoting, capacity filtering, and settlement.

Required: over-capacity request finds no candidate, boundary-weight request, restricted-goods rejection, loading/waiting time affects the final amount correctly, proof captured, duplicate quote request.

# Blocking Conditions

- Waiting-time and loading-time rates undocumented → `BLOCKED_TASKS.md`; these are commercial rates.
- Restricted-goods list undefined → legal and safety decision; ask.
- Liability for damaged cargo unspecified → do not invent a policy.

# Relevant Documentation

`docs/80-cargo-and-vehicle-capacity.md` · `docs/81-loader-rickshaw-and-truck-services.md` · `docs/87-waiting-loading-and-unloading.md` · `docs/88-cargo-job-safety-and-restrictions.md` · `docs/272-cargo-domain.md` · `docs/273-cargo-quoting-and-booking.md` · `docs/274-cargo-dispatch.md` · `docs/275-cargo-documents-and-compliance.md` · `docs/543`–`544` (cargo flows)
