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

# Toolchain

Backend is Go, clients are TypeScript — the commands differ by surface:

- Go (`services/api/`): `go build ./...`, `go vet ./...`, `go test ./<pkg>/...`
- TS (`apps/`, `packages/`): typecheck, lint, and the workspace test runner, scoped to the affected package

**Today the repo contains no application code**, so every task is Level 0 until the foundation lands. Update this section when the toolchain exists.

# Blocking Conditions

- Required test infrastructure does not exist yet → say so plainly and state what was actually verified. Do not describe unrun tests as passing.
- A Level 5 area cannot be tested (no local payment sandbox, no seeded concurrency harness) → block rather than ship unverified financial or dispatch behavior.

# Relevant Documentation

`docs/177-testing-strategy-and-pyramid.md` · `docs/180-e2e-critical-user-journeys.md` · `docs/185-data-consistency-and-idempotency.md` · `docs/333-payment-testing.md` · `docs/334-dispatch-testing.md` · `docs/332-realtime-testing.md`
