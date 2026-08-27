# Implementation Execution Policy

**Status:** Permanent. Governs how implementation work is performed on this
project from Phase 3 onward.

This is the source of truth for implementation efficiency and verification.
`AGENT.md` references it; the implementation skills enforce it. Where this
document and a skill disagree, this document wins and the skill is corrected.

It governs *how* work is done. It does not override *what* is built: the Tier A
documentation (`00`–`190`) remains authoritative for domain and architecture
requirements.

---

## A. Primary Objective

**Implement the product as quickly as possible while maintaining sufficient
engineering quality and correctness.**

Sufficient means: it works, it is tested at a level proportional to its risk, it
follows the documented architecture, and the next person can change it. It does
not mean exhaustive.

Do not optimise for documentation completeness, CI activity, or verification
volume at the expense of implementation velocity. A verified feature beats a
thoroughly documented plan for one. The project has 565 documents and, until
Phase 1, had no code — the correction runs in one direction.

Velocity is not licence to skip the things that are cheap now and expensive
later: tests on money and dispatch, honest status, and the documented
architecture.

---

## B. Documentation Usage

The repository holds ~565 documents. **Do not read them all. Do not read a band
of them.**

Load progressively, stopping as soon as the question is answered:

```text
AGENT.md
    ↓
implementation control documents  (PLAN · STATUS · ADRs · BLOCKED · this policy)
    ↓
the authoritative document(s) owning this task   ← usually 1–3, Tier A
    ↓
the relevant skill
    ↓
the affected source code
```

Read further only when a dependency or a genuine ambiguity demands it.

**Tier map** (established in `DOCUMENT_AUDIT.md`, governed by ADR-005):

| Tier | Range | Treatment |
|---|---|---|
| A | `00`–`190` | Authoritative. Real schemas, state machines, formulas, event names. |
| B | `191`–`204` | Thin restatement. Read the Objective; expect no depth. |
| C | `205`–`564` | Nine template batches of 40. A topic index, not a specification. |

Tier C documents are **not** detailed specifications. Use them to find a topic,
never as the requirement itself — unless a specific one demonstrably contains a
unique requirement, in which case say so explicitly when relying on it.

Never re-read documentation unrelated to the current task. Never re-derive
architecture that `project-discovery` already caches.

---

## C. Implementation Loop

Every implementation task:

1. **Identify** the task and the slice it serves.
2. **Locate** its authoritative documentation (Tier A first).
3. **Identify** dependencies — what must exist before this can work.
4. **Inspect** only the affected code.
5. **Plan** concisely. A few lines, not a design document.
6. **Implement** the smallest complete solution.
7. **Verify** proportionally (§E).
8. **Review the diff** — `git diff`. Look for accidental scope, duplicated
   abstractions, leaked secrets, client-side authority.
9. **Update** `docs/IMPLEMENTATION_STATUS.md` honestly.
10. **Move** to the next unblocked task.

Do not rediscover the architecture at each step. Do not keep analysing once the
required information is known — further analysis at that point is cost without
information.

---

## D. CI Policy

**Remote CI is not a priority during normal implementation.**

The configuration stays in the repository and stays correct. But:

- Do not push solely to make CI run.
- Do not debug CI during feature development.
- Do not spend implementation time optimising CI.
- Do not block implementation on remote CI **unless CI has exposed a real
  implementation defect** — in which case the defect, not CI, is the work.

Remote CI verification is deferred to a milestone or release point.

**Local verification takes priority during active development.** It is faster,
it is under your control, and it catches the same defects.

*Current state:* CI has never executed. Run 33077085568 on pull request #1
(2026-08-27) triggered correctly and every job failed to start — the GitHub
account is locked for billing. This is external to the repository and does not
block implementation.

---

## E. Testing Policy

Testing is required. Testing is **proportional to change impact** — see
`change-impact-analysis` for determining impact and `verification-lite` for the
commands.

| Level | Change | Verification |
|---|---|---|
| 0 | Documentation or configuration with no runtime effect | No application tests. Confirm structured files parse. |
| 1 | Small UI or local change | Targeted check or that component's test. |
| 2 | Component or module internals | That module's unit/component tests. |
| 3 | API surface or domain logic | Targeted unit and integration tests for the affected module. Migrations applied **and** rolled back. |
| 4 | Workflow crossing modules | Integration tests for every module on the path, plus the one E2E journey covering it — when justified. |
| 5 | Payment, ledger, dispatch assignment, auth, concurrency, location privacy, infrastructure | Comprehensive verification of everything the change affects, plus relevant E2E, plus the failure paths: duplicate request, retry, concurrent assignment, partial failure, stale state, unauthorised access. |
| Milestone / release | — | Broader regression verification. |

**Never** run the entire E2E suite automatically after a change.
**Never** run tests the change cannot reach.

Escalation to Level 5 is mandatory, not discretionary, whenever money, dispatch
assignment, auth, or concurrency is touched — regardless of diff size. A
one-line change to fee arithmetic is Level 5.

Evidence or it did not happen: quote the actual output. `IMPLEMENTED` is not
`VERIFIED`.

---

## F. E2E Policy

E2E is expensive and slow. **It is not the default verification mechanism.**

Prefer, in order:

```text
unit  →  component  →  integration  →  targeted E2E
```

Use the smallest layer that gives sufficient confidence.

Run E2E only when a complete user journey changes; multiple independently
deployed components interact; a critical workflow changes; a regression genuinely
cannot be caught at a lower level; or a milestone requires it.

Run the affected journey. Never unrelated scenarios.

*Current state:* no E2E infrastructure exists. When a Level 4 change arrives
before it does, say so plainly rather than implying coverage.

---

## G. Build Policy

Build the affected application or package. Do not rebuild unrelated ones.

Broaden only when dependencies changed, the architecture requires cross-package
verification, or a milestone demands a complete build.

Turborepo already skips unaffected packages and caches unchanged ones, so
`pnpm --filter` is about intent and readable output, not speed. The Go service
is a separate toolchain (ADR-001) and is not rebuilt for a TypeScript change.

---

## H. Architecture Policy

**Follow the documented architecture. Do not redesign it during ordinary
implementation.**

On discovering a genuine conflict:

1. Check the authoritative documentation.
2. Check the existing ADRs.
3. Check implementation dependencies.
4. Determine whether it affects the **current** task.

Does not affect it → record it in `DOCUMENT_CONFLICTS.md` and continue.
Blocks it → record it in `BLOCKED_TASKS.md` and stop that task.

A decision that resolves documented ambiguity or departs from a specification
gets an ADR. Routine choices — naming, file placement, obvious refactors — do
not. Never change architecture silently.

Never invent an undocumented business rule.

---

## I. Business Decisions

Undocumented business rules are catalogued and classified in
`BUSINESS_DECISION_REGISTER.md`. For one that is not:

- Use an existing documented rule if one exists.
- Use a documented technical default where the documentation explicitly permits
  one.
- Otherwise record it as unresolved. Do not guess a commercial, legal or
  financial value.

**Do not stop the project because a future-domain decision is unresolved.** Six
of the nineteen become blocking at the first vertical slice; none block the
foundation. Block only the task that actually requires the decision.

---

## J. Code Quality

Optimise for correctness, maintainability, simplicity, consistency and velocity
— together, not one at the expense of the rest.

Avoid unnecessary abstraction. Avoid speculative infrastructure. Avoid premature
optimisation. **Do not create an interface or framework without a current
consumer.**

Prefer the simplest implementation compatible with the documented architecture.

Conventions the foundation already fixed, which every later slice inherits:

- The error taxonomy and its HTTP mapping live only in `services/api/pkg/httpx`.
- Configuration is loaded and validated once in `pkg/config`; business code
  never reads the environment.
- Migrations run only from `cmd/migrate`, never on startup.
- The backend is authoritative for every business rule. Clients propose; the
  server decides.
- Critical mutations are idempotent and concurrency-safe.
- Tests ship with the implementation, not after it.

---

## K. Token Efficiency

Context is a budget. Do not:

- re-read documentation, in whole or in bands
- repeat a completed audit
- reproduce documentation in responses or in code comments
- re-explain architecture already established this session
- inspect unrelated source trees
- run unrelated tests, unnecessary E2E, or redundant verification

Use targeted context and targeted tooling. Search before reading; read the part,
not the file, when the part will do.

---

## L. Autonomous Execution

**When the task is clearly defined and every required decision is already
documented: proceed.** Do not ask for confirmation of routine engineering
choices the documentation already covers.

**Stop and report when a decision genuinely requires the user's product or
business judgement** — a fee, a rate, a retention period, a liability rule.
State the decision, why it is needed now, and the options. Do not guess, and do
not bury it in a status file.

The distinction is ownership, not difficulty: engineering decisions are yours;
commercial ones are not.

---

## M. Progress Tracking

After completing an implementation task, update
`docs/IMPLEMENTATION_STATUS.md` with the task, its status, a one-line
implementation summary, the verification actually performed, and any blocker.

Keep it to that. **Do not create additional progress documents.** The control
layer is `IMPLEMENTATION_PLAN.md`, `IMPLEMENTATION_STATUS.md`,
`ARCHITECTURE_DECISIONS.md`, `BLOCKED_TASKS.md`,
`BUSINESS_DECISION_REGISTER.md`, `DOCUMENT_CONFLICTS.md` and this policy —
nothing else, unless a new one earns its place.

Status reflects reality, never intent. Downgrade freely when something breaks.

---

## N. Commit Policy

Commit coherent implementation units. Not one commit per file; not one commit
spanning unrelated features.

Conventional commit messages. Explain **why** the change is shaped the way it
is — the diff already shows what changed.

**Do not push unless explicitly instructed.**

---

## O. Stop Conditions

**Stop and report when:**

- the current implementation task is complete
- a genuine blocking architectural decision is required
- a required business decision is missing
- a dangerous ambiguity cannot be resolved safely
- verification exposes a defect that is not resolved

**Do not stop merely because:**

- CI has not run
- unrelated E2E tests have not run
- unrelated documentation has not been reviewed
- future business decisions remain unresolved

---

## Hierarchy

```text
AGENT.md                              mission and protocol
    ↓
IMPLEMENTATION_EXECUTION_POLICY.md    how work is performed  ← this document
    ↓
implementation skills                 the operational loop
    ↓
authoritative documentation           what to build (Tier A)
    ↓
implementation                        smallest complete solution
    ↓
targeted verification                 proportional to impact
```
