# Implementation Plan

Derived from Tier A architecture, **not** from document numbering. Documents `366`–`368` name a
dependency graph, phase list, and work queue but contain none (ADR-005, B-1) — this file is the
working substitute.

## Sequencing Principle

Build vertical slices. One working path through database → backend → API → realtime → client →
tests beats a hundred disconnected tables. `Job` is the hinge: `004` models **all** operational
work as a Job with types `RIDE`, `PARCEL`, `GROCERY`, `CARGO`, `FREIGHT`. Build the Job core once;
the service lifecycles specialize it. Never fork a parallel booking entity per service.

## Dependency Spine

```text
Phase 1  repository foundation ····································· SPECIFIED
             workspace · shared packages · Go foundation
             local infra · CI · testing harness
                    ↓
Phase 2  infrastructure hardening
             (largely folded into Phase 1 for local; cloud is Phase 15)
                    ↓
Phase 3  backend foundation
             migrations · transactions · idempotency · validation
             error mapping · observability · background jobs
             ⚠ BD-07 (money representation) due here
                    ↓
Phase 4  authentication + authorization
             registration · login · sessions · refresh · RBAC · devices
                    ↓
Phase 5  canonical domain
             User · Customer · Provider · Merchant · Vehicle
             VehicleCapability · Job · JobStop · Assignment · Quote
             Payment · Ledger
                    ↓
Phase 6  pricing / quote  →  Phase 7  dispatch  →  Phase 8  location + realtime
                    ↓
Phase 9  FIRST VERTICAL SLICE — RIDE  (reference implementation)
             ⚠ BD-01 … BD-06 due here
                    ↓
Phase 10 delivery · Phase 11 grocery · Phase 12 cargo   (parallel once ride is stable)
                    ↓
Phase 13 financial completeness  →  Phase 14 operations console
                    ↓
Phase 15 production readiness
```

Client foundations (`admin-dashboard`, mobile shells) start in Phase 1 as shells and grow
alongside the backend slices rather than as a separate horizontal phase — a dashboard built before
there is anything to operate is a hundred screens with no system behind them.

## Current Position

**Phase 1, not started.** Repository contains documentation and skills only.

Next task: implement `FIRST_IMPLEMENTATION_SLICE.md`.

## Token-Efficiency Policy

Standing rule from this point forward. Enforced by `project-discovery`, `change-impact-analysis`,
and `verification-lite`.

**Do not:**
- read all 564 documents, or any large band of them
- re-read the architecture that `project-discovery` already caches
- re-run the full E2E suite after a change
- re-run tests the change cannot reach
- rebuild unrelated applications
- perform a full repository analysis for every task

**Default strategy for every task:**

```text
documentation dependency  →  read only the docs owning this task (2–5, Tier A first)
        +
change impact             →  git diff → dependents → affected tests (change-impact-analysis)
        +
targeted verification     →  the level verification-lite assigns, no more
```

**Full verification is reserved for:** major milestones · release candidates · high-risk
cross-cutting changes · payment changes · dispatch and concurrency changes · security-critical
changes · infrastructure changes where warranted.

Escalation to Level 5 remains **mandatory, not discretionary**, whenever money, dispatch
assignment, auth, or concurrency is touched — regardless of how small the diff looks.

## Deferred Decisions

| Item | Due by | Tracked in |
|---|---|---|
| Go ↔ TypeScript shared type strategy | first endpoint (Phase 3) | B-2 |
| Backend module list reconciliation | first domain module (Phase 5) | ADR-004, C-5 |
| Money representation and rounding | before any financial code (Phase 3) | BD-07 |
| Ride commercial rules | Phase 9 | BD-01 … BD-06 |
| Retention policies | before production launch | BD-15, BD-16 |
