# Implementation Status

Reflects reality, not intent. `IMPLEMENTED` ≠ `VERIFIED` — see `progress-tracking`.
**Phase 1 is complete and verified. No product functionality exists.**

`VERIFIED` below means a command was run and its output observed, not that the
code looks right. Evidence is in the Phase 1 completion report.

**Last updated:** 2026-08-27

## Legend

`NOT_STARTED` · `READY` · `IN_PROGRESS` · `BLOCKED` · `IMPLEMENTED` · `VERIFIED` · `DEFERRED` · `OBSOLETE`

## Preparation

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| AGENT.md execution protocol | IMPLEMENTED | n/a | n/a | committed `393d793`, patched `c958f68` |
| Custom skill system (31 skills) | VERIFIED | n/a | YES | frontmatter valid; 228 doc refs resolve |
| Documentation audit | VERIFIED | n/a | YES | signature clustering; `DOCUMENT_AUDIT.md` |
| Document conflict register | IMPLEMENTED | n/a | n/a | 5 conflicts; 4 resolved, 1 deferred |
| Architecture decisions | IMPLEMENTED | n/a | n/a | ADR-001 … ADR-005 |
| Business decision register | IMPLEMENTED | n/a | n/a | 19 items classified |
| Implementation readiness | IMPLEMENTED | n/a | n/a | Phase 1 READY |
| First slice specification | IMPLEMENTED | n/a | n/a | `FIRST_IMPLEMENTATION_SLICE.md` |

## Phase 1 — Repository Foundation

Verified 2026-08-27 against the acceptance criteria in `FIRST_IMPLEMENTATION_SLICE.md`.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| pnpm workspace + Turborepo | VERIFIED | n/a | YES | pnpm 10.33, Turborepo 2.5; 10 build tasks |
| TypeScript config (strict, shared base) | VERIFIED | n/a | YES | strict + `noUncheckedIndexedAccess` + `exactOptionalPropertyTypes`; 17 typecheck tasks pass |
| ESLint + Prettier | VERIFIED | n/a | YES | flat config, re-exported per package; `format:check` clean |
| `@platform/*` package scaffolding (7) | VERIFIED | 15 | YES | no domain types; importability proved from an app by `apps/admin-dashboard/src/workspace.test.ts` |
| Go module + `cmd/api` | VERIFIED | n/a | YES | Go 1.25, own module (ADR-001); `go build ./...` clean |
| Go config loading + validation | VERIFIED | 6 | YES | loaded once at startup; reports every problem at once |
| Go structured logging + request ID | VERIFIED | 4 | YES | JSON; W3C traceparent continued, new span per request |
| Go typed error taxonomy → HTTP | VERIFIED | 6 | YES | single mapping in `pkg/httpx`; cause never serialised |
| Health checks (Postgres/Redis/NATS) | VERIFIED | 9 | YES | 200 healthy → 503 with postgres stopped → 200 on restart |
| Graceful shutdown | VERIFIED | n/a | YES | SIGTERM → in-flight drained → port released |
| Migration runner (explicit command) | VERIFIED | 1 | YES | up → down → up → version 1; never runs on startup |
| Docker Compose local infra | VERIFIED | 3 | YES | four services healthy; integration tests pass against them |
| PostGIS extension enabled | VERIFIED | 1 | YES | `postgis 3.4.3` confirmed in `pg_extension`; startup aborts without it |
| `admin-dashboard` shell (Vite) | VERIFIED | 5 | YES | builds; dev server serves on 5173 with validated env |
| `customer-mobile` shell (Expo) | VERIFIED | 5 | YES | Expo SDK 57; iOS + Android bundles exported; Metro starts |
| `driver-mobile` shell (Expo) | VERIFIED | 5 | YES | as above |
| Environment files + gitignore | VERIFIED | 3 | YES | `.env.local` ignored; public-prefix secret leak rejected at load |
| Testing harness (TS + Go) | VERIFIED | n/a | YES | Vitest, jest-expo, `go test`; integration behind a build tag |
| CI foundation (GitHub Actions) | IMPLEMENTED | n/a | **NO** | YAML valid and affected-surface gated; **not yet observed green on a PR** |
| Root README.md | VERIFIED | n/a | YES | clone-to-running path executed |
| Update `verification-lite` commands | VERIFIED | n/a | YES | real per-surface commands replace the Level-0 placeholder |
| Update `project-discovery` repo state | VERIFIED | n/a | YES | ground truth now describes the built repository |

**Not done, deliberately:** `merchant-dashboard` and `marketing-web` are reserved
directories; `services/api/internal/` is empty; no domain model, endpoint, auth,
NATS subject, WebSocket, native module, E2E harness or Terraform exists.

**Counts:** 28 Go test functions (34 cases with subtests) across 6 packages ·
36 TypeScript tests across 10 packages · 3 integration tests behind a build tag ·
0 E2E, by design.

## Phase 2+ — Not Started

| Phase | Status | Notes |
|---|---|---|
| 2 — infrastructure hardening | NOT_STARTED | local infrastructure landed in Phase 1; cloud is Phase 15 |
| 3 — backend foundation | NOT_STARTED | BD-07 due before financial code |
| 4 — authentication | NOT_STARTED | |
| 5 — canonical domain | NOT_STARTED | ADR-004 to resolve here |
| 6 — pricing / quote | NOT_STARTED | |
| 7 — dispatch | NOT_STARTED | BD-03, BD-04 |
| 8 — location + realtime | NOT_STARTED | BD-15, BD-17 |
| 9 — ride vertical slice | NOT_STARTED | BD-01 … BD-06 |
| 10 — delivery | NOT_STARTED | BD-10, BD-16 |
| 11 — grocery | NOT_STARTED | BD-11, BD-12 |
| 12 — cargo | NOT_STARTED | BD-13 |
| 13 — financial completeness | NOT_STARTED | BD-08, BD-09 |
| 14 — operations console | NOT_STARTED | |
| 15 — production readiness | NOT_STARTED | BD-14, BD-15, BD-19 |

## Blocked

| Item | Blocks | Tracked |
|---|---|---|
| B-1 — control docs 366/367/368 empty | AGENT.md protocol as written | mitigated by `IMPLEMENTATION_PLAN.md`; **Phase 1 unaffected** |
| B-2 — Go ↔ TS type strategy | first endpoint (Phase 3) | `BLOCKED_TASKS.md` — now concrete: the error taxonomy is hand-duplicated in `pkg/httpx/errors.go` and `packages/types/src/errors.ts` |
| B-3 — 19 business rules | Phase 3 onward | `BUSINESS_DECISION_REGISTER.md` |

## Carried Into Phase 2

| Item | Why it matters |
|---|---|
| CI has never run | The workflow is valid YAML with correct steps, but "CI is green" is unproven until a pull request runs it. First PR settles it. |
| Error taxonomy duplicated across Go and TypeScript | Two hand-maintained lists that must agree. Harmless now, expensive when a payment payload drifts. B-2. |
| `postgis/postgis:16-3.4` is amd64-only | Runs under emulation on Apple Silicon. Works, but slower; a multi-arch image is worth evaluating if it bites. |
| MinIO runs with no consumer | Deliberate — the local environment should match the deployed one — but nothing verifies it beyond the container health check. |
