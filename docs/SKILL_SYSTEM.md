# Skill System

Project-specific Claude Code skills for implementing the RideMe logistics platform.
Location: `.claude/skills/<name>/SKILL.md` — 31 skills, 1,823 lines total (~59 avg).

These carry **only** knowledge specific to this platform or this workflow. Generic React,
React Native, TypeScript, Go, PostgreSQL, and Playwright knowledge is deliberately absent —
it is handled by the model and by global skills.

---

## 1. Inventory

### Agent / Execution (8) — foundational

| Skill | Purpose |
|---|---|
| `project-discovery` | Ground truth: locked stack, target layout, what exists, and the documentation tier map. Read first in any session. |
| `documentation-audit` | Classify 565 docs cheaply; record conflicts instead of silently resolving them. |
| `dependency-planning` | Pick the next task by real dependency order — document number is *not* implementation order. |
| `implementation-task` | The build loop: identify → locate docs → inspect → plan → implement → verify → review diff → update status. |
| `progress-tracking` | Honest status in `IMPLEMENTATION_STATUS.md`. `IMPLEMENTED` ≠ `VERIFIED`. |
| `verification-lite` | Proportional verification levels 0–5. The token-efficiency centerpiece. |
| `change-impact-analysis` | What a diff can actually break → feeds the level into `verification-lite`. |
| `architecture-decision` | ADRs for consequential choices, written before implementing. |

### Platform Architecture (6) — foundational

| Skill | Purpose |
|---|---|
| `system-architecture` | Modular monolith, layer boundaries, provider adapters. |
| `domain-modeling` | Canonical entities and state machines; `Job` as the universal work abstraction. |
| `api-contracts` | The ten things every endpoint must define. |
| `event-driven-architecture` | NATS, workers, idempotent consumers, event versioning. |
| `database-architecture` | PostgreSQL/PostGIS ownership, constraints as invariants, migrations, Redis boundary. |
| `realtime-architecture` | WebSocket contracts, subscription authorization, snapshot resynchronization. |

### Logistics Domain (9) — domain-specific

| Skill | Purpose |
|---|---|
| `dispatch-engine` | Candidate filtering, scoring, offers, and the concurrency rule against double-assignment. |
| `location-tracking` | Server-side ingestion, retention, and visibility-as-relationship. |
| `vehicle-service-eligibility` | Capability-based eligibility — never hardcoded vehicle→service mapping. |
| `ride-lifecycle` | The RIDE journey; reference implementation for all other services. |
| `delivery-lifecycle` | PARCEL: proof, multi-stop, failure and return paths. |
| `grocery-lifecycle` | GROCERY: catalog, cart, merchant fulfilment, substitution. |
| `cargo-lifecycle` | CARGO: capacity as hard filter, loading/waiting as billable events. |
| `provider-lifecycle` | Driver onboarding, verification, availability, enforcement. |
| `merchant-lifecycle` | Merchant onboarding, the ownership line, payouts. |

### Financial (3) — domain-specific, all Level 5

| Skill | Purpose |
|---|---|
| `payment-flow` | Intents, capture, refunds, webhooks, COD. Never trust an HTTP 200. |
| `financial-ledger` | Append-only single financial truth; balances derived, never stored mutable. |
| `settlement-reconciliation` | Payouts and reconciliation against provider, bank, and cash records. |

### Mobile (5) — domain-specific

| Skill | Purpose |
|---|---|
| `react-native-platform` | Shared RN+Expo architecture; no forked per-platform screens. |
| `native-module-boundary` | When native is genuinely justified, behind a TS interface. |
| `mobile-location` | Device-side GPS, native filtering/batching, background execution, permissions. |
| `mobile-offline-sync` | Mutation queues, optimistic updates, snapshot recovery. |
| `mobile-performance` | Measure-first; battery is a first-class driver-app metric. |

---

## 2. Dependencies

```text
project-discovery ──> documentation-audit ──> dependency-planning
        │                                            │
        └──────────────> implementation-task <───────┘
                          │        │
      change-impact-analysis    progress-tracking
                          │
                  verification-lite   (every skill routes its Verification here)

system-architecture ──> domain-modeling ──> {api-contracts, database-architecture,
                                             event-driven-architecture, realtime-architecture}
                                │
                    vehicle-service-eligibility ──> dispatch-engine
                                │                        │
                          ride-lifecycle (first slice) <─┘
                                │
        {delivery, grocery, cargo, provider, merchant}-lifecycle

payment-flow ──> financial-ledger ──> settlement-reconciliation

react-native-platform ──> {native-module-boundary, mobile-offline-sync, mobile-performance}
                                  │
                          mobile-location <──> location-tracking (device ↔ server boundary)
```

**Foundational** (load early, apply everywhere): `project-discovery`, `verification-lite`,
`implementation-task`, `change-impact-analysis`, `system-architecture`, `domain-modeling`.

**Domain-specific** (load only when the task is in that domain): everything else.

---

## 3. Verification Strategy

`verification-lite` assigns one of seven levels; `change-impact-analysis` supplies the input.

| Level | Trigger | Response |
|---|---|---|
| 0 | Docs / inert config | No application test |
| 1 | Small local UI | Targeted typecheck + that component's test |
| 2 | Component/module internals | That module's tests |
| 3 | API / domain / schema | Unit + integration; contract tests; migration up-down-up |
| 4 | Cross-module workflow | Path integration tests + **one** E2E journey |
| 5 | Payment, dispatch, security, concurrency, infra | Comprehensive + failure paths |
| Release | Milestone | Full appropriate suite |

Standing prohibitions: never auto-run the full E2E suite after a change; never re-run tests
the change cannot reach; never re-read unrelated documentation to verify.

Escalation is mandatory — not discretionary — whenever money, dispatch assignment, auth, or
concurrency is touched, regardless of diff size. A one-line change to fee arithmetic is Level 5.

Backend is Go (`go build/vet/test`); clients are TypeScript (typecheck/lint/test). **No
application code exists yet**, so everything is Level 0 until the foundation lands.

---

## 4. Token-Efficiency Strategy

1. **The documentation tier map.** `docs/` is 565 files but not uniformly useful:
   - **Tier A (`00`–`190`)** — substantive: schemas, state machines, formulas, event names.
   - **Tier B (`245`–`364`)** — one real Objective line plus generic requirements.
   - **Tier C (`191`–`244`, `365`–`564`)** — ~240 files sharing three near-identical templates;
     unique content per file ≈ one sentence. **Topic index only.**

   Encoded once in `project-discovery`. Without it, an agent reading "authoritative
   specifications" in numeric order burns enormous context on boilerplate and learns nothing.

2. **Reference, never copy.** Skills cite document numbers. No documentation is duplicated
   into a SKILL.md — every fact carries its source so the agent reads only what the task needs.

3. **Progressive loading.** Skill descriptions gate entry; bodies average 59 lines; documents
   load on demand. A typical task touches 2–5 docs.

4. **Proportional verification.** Levels 0–5 prevent the dominant waste: full suites after
   trivial changes.

5. **No architecture rediscovery.** `project-discovery` caches ground truth so no session
   re-derives the stack, layout, or domain model.

---

## 5. Unresolved Gaps

Recorded rather than invented. Each becomes a `docs/BLOCKED_TASKS.md` entry when a task hits it.

### Documentation structure

- **~240 documents are template boilerplate** (`191`–`244`, `365`–`564`). The control documents
  the AGENT.md protocol depends on are among them: `366` names a dependency graph but contains
  none; `367` names phases but lists none; `368` names a work queue but defines none. **The
  documented execution protocol has no machine-readable substrate.** `dependency-planning`
  compensates by deriving order from Tier A architecture instead.
- **Document 11 is absent** from the numbering. A gap, not a missing dependency.

### Architecture conflicts

- **Backend language.** `docs/04` and `docs/12` lock the backend to **Go**; `AGENT.md` Phase 1
  prescribes a TypeScript-wide toolchain. Resolution applied: Go for `services/api/`,
  TypeScript for `apps/` and `packages/`. Recorded, not silently merged.
- **Web framework.** `docs/04` says "Web: Next.js"; `docs/12` splits it (React+Vite for
  admin/merchant, Next.js for marketing). `12` is the later, more specific "Locked Stack" and wins.

### Undocumented business rules

These are product/commercial decisions. No skill invents them:

| Gap | Blocks |
|---|---|
| Cancellation fee amounts and thresholds | `ride-lifecycle` |
| Surge/demand multiplier behaviour | `ride-lifecycle`, pricing |
| Dispatch scoring weights per service type | `dispatch-engine` |
| Behaviour when no dispatch candidate exists | `dispatch-engine` |
| Commission rates, payout schedule, minimum threshold | `financial-ledger`, `settlement-reconciliation`, `merchant-lifecycle` |
| Refund policy: window, partial rules, fee absorption | `payment-flow` |
| Rounding and currency precision | `financial-ledger` |
| Reconciliation discrepancy tolerance and escalation | `settlement-reconciliation` |
| COD liability between driver, merchant, platform | `settlement-reconciliation`, `merchant-lifecycle` |
| Financial consequence of failed delivery; return-leg pricing | `delivery-lifecycle` |
| Grocery substitution price-difference absorption | `grocery-lifecycle` |
| Merchant order acceptance timeout | `merchant-lifecycle` |
| Cargo waiting/loading rates; restricted-goods list; damage liability | `cargo-lifecycle` |
| Required documents per vehicle type; suspension criteria and appeals | `provider-lifecycle` |
| Location retention periods (server and on-device) | `location-tracking`, `mobile-location` |
| Proof-of-delivery photo retention | `delivery-lifecycle` |
| Tracking frequency per job state (battery vs accuracy) | `mobile-location` |
| Offline queue expiry windows; which mutations may queue | `mobile-offline-sync` |
| Mobile performance budgets | `mobile-performance` |

### Adjacent-by-design pairs

Not duplication — a deliberate boundary, cross-referenced in both directions:

- `location-tracking` (server) ↔ `mobile-location` (device)
- `react-native-platform` ↔ `native-module-boundary` (shared TS vs native)
- `dispatch-engine` ↔ `vehicle-service-eligibility` — the eligibility filter set appears in
  both because dispatch applies it and the accept path re-applies it. Both state that it must
  be **one shared implementation**; duplicating the code is how the two drift.
- `change-impact-analysis` → `verification-lite` — a deliberate two-stage pipeline.
