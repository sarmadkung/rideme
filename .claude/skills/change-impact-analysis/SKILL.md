---
name: change-impact-analysis
description: Works out what a diff can actually break, so verification stays narrow and correct. Use before running tests, before deciding whether an E2E run or a full build or a database check is warranted, and when a change touches shared packages or domain contracts.
---

# Purpose

Convert "what did I change?" into "what could that possibly break?" — the input `verification-lite` needs to pick a level.

# When to Use

Between implementing and verifying. Every time.

# Rules

- Start from the diff, not from memory. `git diff --name-only` is the source.
- Impact travels along imports, API contracts, event schemas, and database schema — not along directory adjacency.
- A change to a shared package (`packages/types`, `packages/validation`, `packages/auth`) reaches every consumer. Enumerate them.
- Absence of impact is a claim that needs the same rigor as presence.

# Workflow

1. **What changed** — `git diff --name-only` and `git diff --stat`.
2. **Classify each file**: shared package · backend domain · backend API · worker · realtime · client screen · migration · config · docs.
3. **Trace dependents** — search for imports of changed symbols; for API changes, find the clients calling that endpoint; for event changes, find publishers and subscribers.
4. **Map to tests** — which existing tests execute the changed lines or their dependents.
5. **Answer the four gates** below.
6. **Hand the result to `verification-lite`** as the level input.

# The Four Gates

**Is an E2E run necessary?**
Only if the change alters a user-visible step inside a critical journey, or crosses the client↔server boundary in a way integration tests cannot observe. A pure backend refactor with unchanged contracts: no.

**Is a full build necessary?**
Only if shared types/config changed, dependencies moved, or the build configuration itself changed. Single-module edits: build that module.

**Is database verification necessary?**
Only if a migration, constraint, index, or query changed. Then: apply, verify constraints hold, roll back once, re-apply. Reading a table differently is not a database change.

**Is realtime verification necessary?**
Only if an event name, payload shape, channel, or subscription authorization changed — then test resynchronization and duplicate delivery, since clients must survive both (`docs/389`).

# High-Blast-Radius Surfaces

Changes here reach almost everything; widen scope deliberately:

- `Job` model and `JobStatusChanged` (`docs/15`) — every service lifecycle
- Vehicle capability model (`docs/04`) — dispatch eligibility for all services
- Quote/pricing output — payments and ledger downstream
- Ledger entries (`docs/19`) — irreversible; append-only
- Auth/session/RBAC — every protected route
- WebSocket event contracts (`docs/18`) — both mobile apps and admin

# Verification

Level 0 — analysis changes nothing.

# Blocking Conditions

- Dependents cannot be enumerated (dynamic dispatch, reflection, string-keyed handlers) → say so and escalate the level rather than guessing narrow.
- The change touches a Level 5 surface with no test coverage at all → block; that gap is the task.

# Policy

This skill produces the impact input that §E of
**`docs/IMPLEMENTATION_EXECUTION_POLICY.md`** turns into a verification level.
The four gates above are how §F (E2E restraint) and §G (build scope) are applied
in practice: the default answer to each is *no*, and widening requires a reason
you can state.

# Relevant Documentation

`docs/407-dependency-rules.md` · `docs/406-package-boundaries.md` · `docs/376-event-versioning.md` · `docs/375-api-versioning.md` · `docs/389-realtime-resynchronization.md`
