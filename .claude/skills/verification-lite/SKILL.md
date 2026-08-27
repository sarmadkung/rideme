---
name: verification-lite
description: Assigns proportional verification to a change — the right tests, not all tests. Use before running any test command, before claiming something works, and whenever tempted to run the full E2E suite or full build after a small change.
---

# Purpose

Match verification effort to actual risk. Full suites after every edit waste hours and tokens; skipping tests on payment or dispatch code ships bugs into money and safety paths.

# When to Use

Before every verification step, and before any completion claim.

# The Levels

**Level 0 — docs, comments, config with no runtime effect.**
No application test. Confirm the file parses if it is structured (YAML/JSON/TOML).

**Level 1 — small local UI or copy change, one component, no shared contract.**
Targeted typecheck plus that component's existing test if one exists.

**Level 2 — component or module internals, no API or schema change.**
That module's unit/component tests.

**Level 3 — API surface, domain logic, or schema change.**
Unit plus integration tests for the affected module. Contract tests if the API shape moved. Migration applied and rolled back once locally.

**Level 4 — workflow crossing modules (booking→dispatch, order→delivery, client↔realtime).**
Integration tests for every module on the path, plus the one E2E journey that covers it. One journey — not the suite.

**Level 5 — payment, ledger, dispatch assignment, auth/authz, concurrency, location privacy, or infrastructure.**
Comprehensive tests for everything affected, plus relevant E2E, plus the failure paths that make these areas dangerous: duplicate request, webhook retry, concurrent assignment, partial failure, stale state, unauthorized access.

**Release / milestone.** Full appropriate suite. Only here.

# Rules

- Never auto-run the entire E2E suite after a change. Choose the journey.
- Never re-run tests the change cannot reach.
- Never re-read unrelated documentation to verify.
- Escalate one level when unsure between two. Escalate to 5 whenever money, dispatch assignment, auth, or concurrency is touched, regardless of diff size — a one-line change to fee arithmetic is Level 5.
- Evidence or it did not happen. Paste the actual counts and the pass/fail line.

# Commands By Surface

The repository holds two toolchains (ADR-001). Pick by what the change touched.

| Surface | Typecheck / vet | Lint | Test | Build |
|---|---|---|---|---|
| One TS package | `pnpm --filter @platform/<name> typecheck` | `... lint` | `... test` | `... build` |
| One TS app | `pnpm --filter @apps/<name> typecheck` | `... lint` | `... test` | `... build` |
| Whole workspace | `pnpm typecheck` | `pnpm lint` | `pnpm test` | `pnpm build` |
| One Go package | `go vet ./pkg/<name>/...` | `gofmt -l pkg/<name>` | `go test ./pkg/<name>/...` | `go build ./...` |
| Whole Go service | `go vet ./...` | `make api-lint` | `go test -race ./...` | `go build ./...` |
| Everything | — | — | — | `make verify` |

Go commands run from `services/api/`. Turborepo already skips unaffected
packages and caches unchanged ones, so a scoped `--filter` is about intent, not
speed — but it keeps the output readable.

**Infrastructure-dependent checks** (need `make infra-up` first):

- `go test -tags=integration ./tests/...` — Postgres/PostGIS, Redis, NATS reachable
- `make migrate-up && make migrate-down && make migrate-up` — migrations reversible
- `curl -s localhost:8080/health | jq` — with a dependency stopped, must return 503

# What Each Level Actually Runs

**Level 0** — `pnpm format:check`, or nothing. YAML/JSON parses.

**Level 1** — the one package's `typecheck` and `test`.

**Level 2** — that package's `test`, plus `typecheck` on packages importing it
(Turborepo resolves this: `pnpm --filter ...@platform/<name>... test`).

**Level 3** — the Go package's `go test`, plus `-tags=integration` if it touches
Postgres, Redis or NATS. A migration must be applied **and rolled back** locally.

**Level 4** — integration tests for every module on the path, plus the single
E2E journey covering it. No E2E infrastructure exists yet; when a Level 4 change
arrives before it does, say so rather than claiming coverage.

**Level 5** — `make verify`, plus `-race`, plus the failure paths listed above.
Infrastructure changes (`docker-compose.yml`, CI, Makefile) are Level 5: bring
the stack down and up from clean, confirm every service reachable, migrations
reversible, health accurate in both directions.

# Blocking Conditions

- Required test infrastructure does not exist yet → say so plainly and state what was actually verified. Do not describe unrun tests as passing.
- A Level 5 area cannot be tested (no local payment sandbox, no seeded concurrency harness) → block rather than ship unverified financial or dispatch behavior.

# Relevant Documentation

`docs/177-testing-strategy-and-pyramid.md` · `docs/180-e2e-critical-user-journeys.md` · `docs/185-data-consistency-and-idempotency.md` · `docs/333-payment-testing.md` · `docs/334-dispatch-testing.md` · `docs/332-realtime-testing.md`
