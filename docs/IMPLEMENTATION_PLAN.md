# Implementation Plan

Derived from Tier A architecture, **not** from document numbering. Documents `366`–`368` name a
dependency graph, phase list, and work queue but contain none (ADR-005, B-1) — this file is the
working substitute.

> **Phase ordering is now governed by `MASTER_IMPLEMENTATION_ROADMAP.md`.** The dependency
> spine below remains the reasoning behind it; the roadmap holds the executed order and a
> translation table between the two numberings.
>
> Two things below are superseded by owner decisions of 2026-08-28 (C-6 / R-2, R-5):
> **pricing has no phase of its own** — it is a shared capability (CAP-1) whose boundary is
> created by the ride slice; and the "clients grow alongside the slices" principle is
> **upheld**, with Phases 12 and 13 acting as consolidation phases rather than horizontal
> builds. Four further capabilities — maps/ETA, safety/fraud, notifications, analytics — are
> staged as cross-cutting tracks (CAP-2 … CAP-5) inside the existing 15 phases.

## Sequencing Principle

Build vertical slices. One working path through database → backend → API → realtime → client →
tests beats a hundred disconnected tables. `Job` is the hinge: `004` models **all** operational
work as a Job with types `RIDE`, `PARCEL`, `GROCERY`, `CARGO`, `FREIGHT`. Build the Job core once;
the service lifecycles specialize it. Never fork a parallel booking entity per service.

## Dependency Spine

```text
Phase 1  repository foundation ····································· COMPLETE
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

**Phase 1 complete and verified (2026-08-27).** The repository runs end to end:
workspace resolves, Go API starts and proves every dependency, local
infrastructure comes up under Docker, migrations apply and roll back, all
quality gates pass. No product functionality exists. Evidence per task is in
`IMPLEMENTATION_STATUS.md`.

One criterion is outstanding and cannot be closed locally: remote CI has never
executed. Run 33077085568 on pull request #1 was triggered correctly on
2026-08-27 and every job failed to start — the GitHub account is locked for
billing. This is external to the repository. Under
`IMPLEMENTATION_EXECUTION_POLICY.md` it does not block implementation; remote CI
verification is a milestone activity, not a per-change gate.

**Roadmap Phase 2 (contracts) completed 2026-08-28** — B-2 closed by ADR-007, BD-07
implemented as ADR-008, CAP-5 envelope in place. Next is roadmap Phase 3 (identity
and auth). The paragraph below describes the same work under the old numbering.

**Next: Phase 3 — backend foundation.** Phase 2 as originally drawn was
infrastructure hardening; its local half landed inside Phase 1 (Docker, health,
observability, error taxonomy, migration mechanism) and its cloud half belongs
to Phase 15. It is not a separate unit of work, and pretending otherwise would
manufacture a phase with nothing in it.

Phase 3 opens with two things that must be settled first, both already tracked:

- **B-2 — Go ↔ TypeScript type strategy.** Now concrete rather than
  hypothetical: the error taxonomy already exists twice, hand-maintained, in
  `pkg/httpx/errors.go` and `packages/types/src/errors.ts`. Decide the source of
  truth before a third contract is duplicated.
- **BD-07 — money representation and rounding.** Due before any code touches an
  amount. A product decision, not an engineering one.

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
| ~~Go ↔ TypeScript shared type strategy~~ | **resolved 2026-08-28** | ADR-007 |
| Backend module list reconciliation | first domain module (Phase 5) | ADR-004, C-5 |
| ~~Money representation and rounding~~ | **resolved 2026-08-28** | ADR-008 |
| Ride commercial rules | Phase 9 | BD-01 … BD-06 |
| Messaging/OTP capability | **roadmap Phase 3 — required by `020`/`028`** | CAP-4, C-6 |
| Retention policies | before production launch | BD-15, BD-16 |
