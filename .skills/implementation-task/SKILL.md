---
name: implementation-task
description: The standard loop for building one unit of RideMe work — locate docs, inspect code, plan minimally, implement, verify proportionally, update status. Use for every implementation task, and especially when you notice yourself re-reading architecture you already established this session.
---

# Purpose

Turn a task into working code in one pass, without rediscovering the platform each time.

# When to Use

Every implementation unit. If you are about to write or change application code, you are in this loop.

# Policy

This loop implements §C of **`docs/IMPLEMENTATION_EXECUTION_POLICY.md`**, under
§A: implement as quickly as possible while keeping quality sufficient. Read the
policy for documentation loading (§B), code quality (§J), autonomous execution
(§L) and stop conditions (§O). It is not restated here.

# Rules

- Context established earlier in the session is still valid. Do not re-derive the stack, the layout, or the domain model — `project-discovery` holds them.
- Read only the docs that own the current task. Two to five is normal; twenty is a mistake.
- Reuse before creating. Search the target module first.
- Backend is authoritative for every business rule. Clients propose; the server decides.
- Critical mutations are idempotent and concurrency-safe (`docs/377`, `docs/378`).
- Tests ship with the implementation, not after it.
- Smallest correct implementation. No speculative extensibility.

# Workflow

1. **Identify** the task and the slice it serves (`dependency-planning`).
2. **Locate** authoritative docs — Tier A first; Tier C only for the topic title.
3. **Inspect** the affected module for existing code, types, and tests.
4. **Plan** minimally: what changes, what tests prove it, what could break (`change-impact-analysis`).
5. **Implement** the slice — schema → domain → API → realtime → client, as the task requires.
6. **Verify** at the level `verification-lite` assigns. Not more, not less.
7. **Review the diff** — `git diff`. Look for accidental scope, duplicated abstractions, leaked secrets, client-side authority.
8. **Update** `docs/IMPLEMENTATION_STATUS.md` with the honest status.
9. **Continue** to the next unblocked task, or stop if blocked.

Do not stop because CI has not run, unrelated E2E has not run, or a
future-domain business decision is unresolved (policy §O).

# Verification

Assigned per task by `verification-lite`. Never claim a status you did not observe: quote the command output.

# Blocking Conditions

Stop and write `docs/BLOCKED_TASKS.md` when the task requires a business rule the docs do not state, a destructive migration, an unavailable credential or external service, or a genuine architecture decision. Record: task, reason, relevant docs, decision needed, options, recommendation. Do not invent the requirement.

# Relevant Documentation

`docs/377-idempotency-standard.md` · `docs/378-concurrency-control.md` · `docs/379-transaction-boundaries.md` · `docs/374-api-error-standard.md` · `docs/370-coding-standards.md` · `docs/564-agent-workflow-execution-spec.md`
