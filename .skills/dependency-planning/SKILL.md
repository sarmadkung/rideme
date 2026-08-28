---
name: dependency-planning
description: Chooses the next implementation task by real dependency order rather than document number. Use when selecting what to build next, when sequencing a phase, or when tempted to implement documents in numeric order — which is wrong for this repo.
---

# Purpose

Order work by what must exist first, not by filename.

# When to Use

- Picking the next unblocked task.
- Sequencing a phase or vertical slice.
- Judging whether a task's prerequisites are genuinely satisfied.

# Rules

- **Document number is not implementation order.** `docs/366` names a dependency graph but contains none — it is Tier C boilerplate. Derive order from architecture instead.
- Build vertical slices, not horizontal layers. One working path beats 100 tables.
- A prerequisite is satisfied only when its code exists and passes verification — not when its document has been read.
- Prefer the smallest slice that produces observable behavior end to end.

# Real Dependency Spine

Derived from `docs/04`, `09`, `12`, `13`, `15`:

```text
repo foundation (workspace, Go module, TS config, CI)
        ↓
infrastructure (Postgres+PostGIS, Redis, NATS, object storage — local)
        ↓
backend foundation (config, logging, errors, validation, migrations,
                    transactions, idempotency, health, observability)
        ↓
identity + auth  →  authorization/RBAC
        ↓
canonical domain (User, Customer, Provider, Vehicle, VehicleCapability,
                  Job, JobStop, Assignment, Quote, Payment, Ledger)
        ↓
pricing/quote  →  dispatch  →  location + realtime
        ↓
first vertical slice: RIDE  (see ride-lifecycle)
        ↓
delivery · grocery · cargo   (parallel once ride is stable)
        ↓
financial completeness · operations console · production readiness
```

`Job` is the hinge: `docs/04` models **all** operational work as a Job with types RIDE, PARCEL, GROCERY, CARGO, FREIGHT. Build the Job core once; the service lifecycles specialize it. Never fork a parallel booking entity per service.

# Workflow

1. State the candidate task and the slice it belongs to.
2. Walk the spine upward — is every layer above it implemented and verified?
3. If not, the true next task is the highest unimplemented prerequisite.
4. Confirm no existing code already covers it.
5. Record the choice and its rationale in `docs/IMPLEMENTATION_PLAN.md`.

# Verification

Level 0 for planning. The task itself carries its own level.

# Blocking Conditions

- Two candidate tasks each require the other → circular; record in `docs/BLOCKED_TASKS.md`.
- A prerequisite needs a product decision the docs do not make → stop; do not invent the rule.

# Relevant Documentation

`docs/04-domain-architecture.md` · `docs/09-project-structure.md` · `docs/22-engineering-phases.md` · `docs/08-roadmap.md` · `docs/366`–`368` (Tier C — intent only)
