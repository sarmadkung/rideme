# Implementation Readiness

**Date:** 2026-08-27 · **Question:** can Phase 1 begin?

## Verdict

**READY to begin Phase 1.**

The engineering foundation is specified in unusual detail by documents `012`, `023`, `024`, `025`,
and `013`. Nothing about the foundation slice depends on an unresolved business decision.

The 19 open business rules in `BUSINESS_DECISION_REGISTER.md` are real, but **none of them blocks
Phase 1** — the earliest becomes live in Phase 3 (money representation) and the bulk at the first
vertical slice. Declaring the project blocked on them would be wrong.

---

## Assessment

### READY (9)

| Area | Status | Evidence |
|---|---|---|
| **Architecture** | READY | `004`, `012`, `191` — modular monolith, extraction boundaries defined. Consistent across Tier A. |
| **Repository structure** | READY | `023` gives the complete tree, pnpm workspaces, Turborepo. `009` corroborates. Naming conflict resolved in ADR-003. |
| **Backend technology** | READY | Go, locked by `012`. `025` specifies module layout, layering, error taxonomy, transaction and concurrency rules. ADR-001 settles the toolchain split. |
| **Frontend technology** | READY | React + Vite for dashboards, Next.js for marketing only. ADR-002. |
| **Mobile technology** | READY | React Native + Expo + TypeScript. `017` defines the native boundary and the `LocationService` interface. |
| **Database** | READY | PostgreSQL + PostGIS. `013` gives core tables and columns; `024` gives the local database and PostGIS setup. |
| **Infrastructure (local)** | READY | `024` specifies Docker Compose services (postgres, redis, nats, minio), ports, and env var names. |
| **API conventions** | READY | `025` defines the handler contract and typed error taxonomy mapped to HTTP. Sufficient for the foundation; endpoint-level conventions firm up in Phase 3. |
| **Domain model** | READY | `004`, `013`, `015`, `016` — canonical entities, Job abstraction, state machines. Not needed in Phase 1 but unblocked for Phase 5. |

### BLOCKED (0)

Nothing blocks Phase 1.

Two items block *later* phases and are tracked in `BLOCKED_TASKS.md`:

- **B-2 — Go ↔ TypeScript shared type strategy.** Must be settled before the first endpoint (Phase 3), not before Phase 1.
- **B-3 — business rules.** First bite in Phase 3 (BD-07, money representation), then heavily at the ride slice.

### NOT_REQUIRED_YET (8)

Deliberately deferred. Listing them as blockers would misrepresent readiness.

| Area | Why not yet | Needed by |
|---|---|---|
| **Authentication** | Phase 4. `020`, `205`–`208` exist; no auth surface in the foundation slice. | Phase 4 |
| **Event model** | NATS connectivity is proven in Phase 1; event *contracts* (`018`, `049`) come with the first async workflow. | Phase 3 |
| **Testing strategy** | `177` defines the pyramid. Phase 1 needs test infrastructure to *exist and run*, not a full suite. | Phase 1 (harness only) |
| **CI/CD** | `023` and `170` define the PR pipeline: install, lint, typecheck, unit tests, build. Phase 1 stands this up; deployment pipelines are Phase 15. | Phase 1 (CI only) |
| **Environment management** | `023`, `024` specify `.env.example` / `.env.local` / `.env.test` and the variable names. Secrets management proper is Phase 15. | Phase 1 (local only) |
| **Observability** | `025` requires request ID, structured logs, latency, status, trace context at the backend foundation. Sentry/OTel wiring is Phase 3+. | Phase 3 |
| **Security** | `202`, `314`–`318` exist. Phase 1 security is limited to: no secrets committed, `.env` ignored, dependencies pinned. Real surface arrives with auth. | Phase 4 |
| **Unresolved business decisions** | 19 items, none Phase 1. See the register's timeline table. | Phase 3 onward |

---

## Risks Carried Into Phase 1

1. **The documented control layer is empty.** `366`–`368` contain no dependency graph, phases, or
   work queue (ADR-005, B-1). Sequencing uses the derived spine in `IMPLEMENTATION_PLAN.md`.
   Mitigation: the spine is grounded in Tier A architecture and is written down.

2. **Two toolchains.** Go and TypeScript verified separately (ADR-001). Mitigation: `verification-lite`
   selects commands by surface; CI runs both paths.

3. **`verification-lite` currently reports Level 0 for everything**, because no application code
   exists. It must be updated in the Phase 1 commit that first makes tests runnable — otherwise it
   silently under-verifies from Phase 2 onward. **This is the single most important follow-up.**

4. **Tier C titles may imply requirements that were never specified.** A title like
   `461-tax-engine` names a real concern with no content behind it. Treat such titles as questions
   for the owner, not as scope.

---

## Recommendation

Proceed to Phase 1 as scoped in `FIRST_IMPLEMENTATION_SLICE.md`.

Raise BD-07 (money representation) before Phase 3 begins, and BD-01 through BD-06 before the ride
slice. Everything else can wait without stalling engineering.
