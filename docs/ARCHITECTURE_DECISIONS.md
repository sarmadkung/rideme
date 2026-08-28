# Architecture Decision Records

Decisions that resolve documented ambiguity or depart from a specification. Routine choices
(naming, file placement, obvious refactors) are not recorded here.

---

## ADR-001 — Go backend outside the JavaScript workspace

**Date:** 2026-08-27 · **Status:** Accepted

**Context:** `AGENT.md` Phase 1 prescribes a repository-wide TypeScript toolchain, while `004`
and `012` lock the backend to Go. `023` states explicitly: "Do not force Go into the JavaScript
workspace." Conflict C-1.

**Decision:** `services/api/` is an independent Go application with its own `go.mod`, verified by
`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt`. The pnpm/Turborepo workspace covers
`apps/` and `packages/` only. AGENT.md's "typecheck / lint / test / build" is interpreted
per-surface.

**Alternatives:** (a) Node/TypeScript backend — contradicts the locked stack and discards the Go
concurrency model that dispatch depends on. (b) Go inside the pnpm workspace via wrappers — adds
indirection with no benefit and is explicitly warned against by `023`.

**Consequences:** Two toolchains and two CI paths. `verification-lite` must select commands by
surface. Shared types between Go and TypeScript are duplicated or generated — an open question
deferred to the first API slice, not Phase 1.

**Affects:** repository root, CI, `verification-lite`.

---

## ADR-002 — React + Vite for dashboards; Next.js for marketing only

**Date:** 2026-08-27 · **Status:** Accepted

**Context:** `004` says "Web: Next.js"; `012` and `023` scope Next.js to marketing and specify
React + Vite for the operational dashboards. Conflict C-2.

**Decision:** `merchant-dashboard` and `admin-dashboard` are React + Vite + TypeScript.
`marketing-web` is Next.js + TypeScript. Next.js is not introduced elsewhere.

**Alternatives:** Next.js everywhere — SSR buys little for authenticated operational consoles and
costs build complexity; contradicts the later Locked Stack.

**Consequences:** Dashboards are client-rendered SPAs; SEO is irrelevant for them. Two web build
systems exist, but in separate applications with no shared build config.

**Affects:** `apps/merchant-dashboard`, `apps/admin-dashboard`, `apps/marketing-web`.

---

## ADR-003 — Application directory names follow document 023

**Date:** 2026-08-27 · **Status:** Accepted

**Context:** `009` and `023` disagree on two directory names. Conflict C-3.

**Decision:** `apps/customer-mobile`, `apps/driver-mobile`, `apps/merchant-dashboard`,
`apps/admin-dashboard`, `apps/marketing-web`.

**Alternatives:** `009`'s `merchant-web`/`admin-web` — the earlier, less specific document.

**Consequences:** `009`'s tree is superseded for naming. Its backend module list and
provider-interface rule remain authoritative.

**Affects:** repository structure.

---

## ADR-004 — Backend module list reconciliation deferred → resolved

**Date:** 2026-08-27 · **Resolved:** 2026-08-28 · **Status:** Accepted · **Resolves:** C-5

**Context:** `009` and `025` list different backend modules; `025` adds `tracking` and omits
`wallet`, `ratings`, `zones`. Those three have their own Tier A documents, so the omission reads
as abbreviation rather than a decision. Conflict C-5.

**Decision:** Adopt `025`'s directory layout and layering now. Treat the module list as open;
add `wallet`, `ratings`, `zones`, and `tracking` when their slices are built.

**Resolution (2026-08-28, roadmap Phase 4 — the first domain module).** The tie is broken by
`004`, which was not consulted when this ADR was written. `004`'s "Core Domains" list is
`009`'s exactly — it includes `wallet`, `ratings` and `zones`. Two Tier A documents against
one, and `004` is the master architecture.

**The module list is the union:** `009`/`004`'s seventeen domains plus `tracking` from `025`.
`025` remains authoritative for *structure* — the directory layout and the
handler → application → domain → repository layering — which was never in dispute. Its
omission of three domains reads as abbreviation, not decision: each has its own Tier A
document (`53-wallets-and-ledger`, `97-geofencing-and-zone-model`,
`111-ratings-reviews-and-quality`) and none could be dropped without losing documented
behaviour.

Modules are created when their slice is built, not up front. `internal/identity` (Phase 3)
and `internal/jobs` (Phase 4) exist; the remaining sixteen are directories that do not yet
exist and should not be created empty.

**Consequences:** C-5 is closed. `jobs` is the module that matters most and it is deliberately
one module for all five job types — `004`'s Job abstraction means there is no `rides` module,
no `parcels` module, and there must never be one.

**Affects:** `services/api/internal/`.

---

## ADR-005 — Documentation tier map governs reading strategy

**Date:** 2026-08-27 · **Status:** Accepted

**Context:** 360 of 564 documents (205–564) are nine 40-document template batches whose only
unique content is a title and a one-sentence Objective. `366`, `367`, and `368` — the dependency
graph, phases, and work queue that AGENT.md's protocol depends on — are among them and contain
no graph, no phases, and no queue. See `DOCUMENT_AUDIT.md`.

**Decision:** Treat 000–190 as authoritative specification, 191–204 as thin restatement, and
205–564 as a topic index. Derive implementation order from Tier A architecture
(`dependency-planning`) rather than from `366`–`368`. Encode the map in `project-discovery`.

**Alternatives:** Follow the documented protocol literally — burns ~110,000 words of boilerplate
and yields no dependency information. Regenerate `366`–`368` with real content — that is authoring
new specification, which is the owner's call, not the agent's.

**Consequences:** The documented control layer is bypassed for sequencing. If `366`–`368` are ever
given real content, this ADR should be superseded. The tier map must be re-verified if documents
are added or rewritten.

**Affects:** `project-discovery`, `dependency-planning`, `documentation-audit`, `IMPLEMENTATION_PLAN.md`.

---

## ADR-006 — Two test runners: Vitest for web and packages, jest-expo for mobile

**Date:** 2026-08-27 · **Status:** Accepted

**Context:** `177` names the testing layers but not the runners. React Native
cannot be tested by Vitest without reimplementing what `jest-expo` already
provides (the Metro transform, the RN module registry, native mocks), and Expo
ships and supports only the Jest preset. Meanwhile Vitest is the natural runner
for Vite, and forcing Jest onto the dashboards would mean maintaining a second
transform pipeline for no gain.

**Decision:** `apps/*-mobile` use `jest-expo` (Jest 29 — the line `jest-expo` 57
is built on). Everything else uses Vitest. Both are invoked through the same
`pnpm test` / Turborepo task, so the split is invisible from the command line.

`@platform/*` packages publish ESM only. Jest 29 runs CommonJS, so the mobile
apps map `@platform/<name>` to the package's `src/index.ts` and let
`babel-preset-expo` transform it. Mobile tests therefore exercise source, not
built output; `pnpm build` verifies the built output separately.

**Alternatives:** (a) Jest everywhere — a second transform pipeline for the web
apps, and slower. (b) Vitest everywhere — would require hand-maintaining a React
Native environment that Expo already ships. (c) Dual CJS/ESM builds for every
shared package — real complexity in exchange for removing a three-line
`moduleNameMapper`.

**Consequences:** Two runners means two sets of matchers and two config shapes;
a test helper written for one does not work in the other. Accepted. If a mobile
test ever passes against source while the built package is broken, `pnpm build`
is the check that catches it — which is why build stays a required CI step
rather than an optional one.

**Affects:** `apps/*-mobile`, `apps/admin-dashboard`, `packages/*`, CI.

---

## ADR-007 — Go is authoritative for the wire contract; TypeScript is generated

**Date:** 2026-08-28 · **Status:** Accepted · **Resolves:** B-2

**Context:** `023` specifies `@platform/types` for shared types and
`@platform/validation` for Zod schemas; `025` specifies Go domain entities.
Neither says how the two stay in sync. Phase 1 shipped with the consequence:
one error taxonomy existed three times — Go constants in `pkg/httpx`, a
TypeScript union in `@platform/types`, a Zod enum in `@platform/validation` —
each hand-maintained, with no domain payloads yet. Three copies of one envelope
before the first endpoint. B-2.

**Decision:** The Go types in `services/api` are the single source of truth for
every shape the API puts on the wire. `packages/types/src/generated.ts` and
`packages/validation/src/generated.ts` are generated from them by
`services/api/cmd/contractgen`, which reflects over the registered Go structs.
`make contracts` regenerates; `make contracts-check` fails when the checked-in
output is stale and runs inside `make verify`.

Hand-written TypeScript may not declare a wire shape. It may add what
generation cannot: runtime guards over generated values, and display
formatting.

**Why generate from Go rather than from OpenAPI.** OpenAPI would be a third
artifact to keep true, and its guarantee is only as good as the hand-written
spec. The Go type is what actually serializes the response — generating from it
means the emitted TypeScript cannot describe a shape the server does not serve.
Drift is not detected; it is unrepresentable.

**Why reflection rather than a code-generation framework.** The registry is one
Go file with no dependencies outside the standard library. It understands the
shapes this platform puts on the wire and **fails generation** on anything else,
so an unmappable type is a build error rather than an approximation. The
execution policy forbids speculative infrastructure; a schema compiler for a
contract of nine types would be exactly that.

**Alternatives:** (a) Keep three hand-maintained copies — the status quo that
produced this ADR; drift in a financial or dispatch payload is expensive and
silent. (b) OpenAPI as the contract, generating both sides — the strongest
guarantee on paper, but the largest setup and a third artifact that can itself
be wrong. Revisit if the API is ever consumed by a client outside this
repository, which is the case OpenAPI actually answers.

**Consequences:** A contract change is a Go change plus `make contracts`. The
registration list in `cmd/contractgen` is still written by hand and is
guarded by tests that fail when a Go constant is added without being
registered. Clients cannot introduce a payload type unilaterally — correct, as
the backend is authoritative for every contract.

**Affects:** `services/api/pkg/contract`, `services/api/cmd/contractgen`,
`packages/types`, `packages/validation`, `Makefile`, CI.

---

## ADR-008 — Money is an integer count of minor units, bounded by JavaScript's safe range

**Date:** 2026-08-28 · **Status:** Accepted · **Implements:** BD-07

**Context:** BD-07 is classified `TECHNICAL_DEFAULT` and recommends integer
minor units in a single currency, rounded once at the final customer-facing
amount, half-up. It is due before any code touches an amount. `019` shows whole
rupees and states no representation; `451` is Tier C and states nothing.

**Decision:** Adopt the register's recommendation, explicitly.
`services/api/pkg/money` holds `Amount{Minor int64, Currency}`. No float64
appears in any monetary path. Rounding happens in exactly one place —
`ApplyRate`, which takes a rate as an integer numerator and denominator and
rounds half away from zero — so "round once, at the end" is enforceable rather
than aspirational. `Allocate` splits an amount so the parts sum to exactly the
whole, because a remainder dropped in a ledger is an unexplained imbalance.

Amounts are additionally bounded to ±`MaxSafeMinor`
(9,007,199,254,740,991 — JavaScript's `Number.MAX_SAFE_INTEGER`), on both
sides of the wire. Beyond it a TypeScript client silently loses precision on a
value the server still holds exactly; rejecting is the only behaviour that
keeps the two in agreement. The bound is ~90 trillion PKR and constrains
nothing real.

**Clients do no money arithmetic.** `@platform/types` exports the generated
`Money` type and `formatMoney` for display, and nothing else. A fare added up
in a browser is a second implementation of a rule that already lives on the
server.

**Alternatives:** (a) Decimal string on the wire — avoids the JS bound entirely
but makes every client parse before it can compare, and invites a float on the
other side of that parse. (b) `numeric` all the way through with a big-decimal
type — heavier than a single-currency platform in paisa needs. (c) Serialise
the integer as a string — safe past the bound, at the cost of every consumer
converting; the bound makes it unnecessary.

**Consequences:** Currency is carried on every amount even though the platform
is single-currency, so a second currency is a schema change rather than a
silent reinterpretation of every stored integer. Rates are integers: a 12.5%
commission is `ApplyRate(125, 1000)`, never `0.125`. This ADR fixes the
representation only — **no rate, fee, commission or fare value is encoded
anywhere.** BD-01, BD-02, BD-05 and BD-13 remain open.

**Affects:** `services/api/pkg/money`, `packages/types`, `packages/validation`,
and every phase from 7 onward that touches an amount.

---

## ADR-009 — List responses are cursor-paginated

**Date:** 2026-08-28 · **Status:** Accepted

**Context:** The Phase 2 roadmap requires pagination conventions. No Tier A
document specifies a pagination strategy — `178` names pagination only as a
testing topic. This is an engineering decision with no documented answer, so it
is recorded rather than assumed.

**Decision:** List responses carry `PageInfo{next_cursor, limit}`. The cursor is
opaque: clients pass it back unmodified and never construct or parse one.
`limit` defaults to 25 and is capped at 100; a request above the cap is clamped
rather than rejected.

**Why cursor rather than offset.** The platform's lists are time-ordered and
actively written to — jobs, driver locations, ledger entries. An offset into a
list that is growing at the head skips and repeats rows as it shifts, which for
a ledger means an operator paging through entries can miss one. A cursor does
not have that failure mode.

**Alternatives:** Offset/limit — simpler to implement and to jump within, and
wrong for exactly the lists that matter most here.

**Consequences:** Endpoints cannot cheaply offer "jump to page N". No list
endpoint exists yet, so nothing is migrated. Cursor encoding is decided by the
first list endpoint, in Phase 3 or later; only the envelope is fixed here.

**Affects:** `services/api/pkg/httpx`, `packages/types`, every list endpoint.


---

## ADR-010 — One state machine engine, three documented machines

**Date:** 2026-08-28 · **Status:** Accepted

**Context:** Document 15 defines the job lifecycle, document 16 defines the driver and vehicle
lifecycles, and each says transitions are performed by backend commands rather than assigned by
clients. More machines arrive with assignments, payments, orders and merchant fulfilment.

**Decision:** `pkg/statemachine` holds a small table-driven engine; each domain declares its
machine as data. A transition is validated against the declaration, and refused transitions
carry the states that *were* allowed.

Every transition additionally uses **compare-and-set** at the database — `WHERE status = $from`
— rather than a plain UPDATE.

**Why compare-and-set is not optional.** Two actors racing on one job is the normal case, not
the edge case: a customer cancels while dispatch assigns. Both read the same status, both
consider their move legal, and with a plain UPDATE both writes land — the second silently
overwriting the first. That produces a cancelled job with a driver on the way, and no error
anywhere. The predicate makes exactly one win and tells the loser it lost.

**Alternatives:** (a) A hand-written `switch` per domain — the same logic three times, drifting
independently, with no shared way to report what was allowed. (b) Serializable transactions —
correct, and far more expensive than one predicate on a primary-key update. (c) Advisory locks
— an extra round trip and a lock to leak.

**Consequences:** A machine is declared once and its shape is testable without a database, so
"can a cancelled job be resurrected?" is a unit test. Terminal states are part of the
declaration rather than a hardcoded list at each call site. Callers must supply the status they
believe the entity is in, which is slightly more work and is exactly what makes the race
detectable.

**Affects:** `pkg/statemachine`, `internal/jobs`, and every module with a lifecycle from Phase 5
onward.
