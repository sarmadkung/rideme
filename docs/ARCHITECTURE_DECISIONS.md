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

## ADR-004 — Backend module list reconciliation deferred

**Date:** 2026-08-27 · **Status:** Proposed (deferred)

**Context:** `009` and `025` list different backend modules; `025` adds `tracking` and omits
`wallet`, `ratings`, `zones`. Those three have their own Tier A documents, so the omission reads
as abbreviation rather than a decision. Conflict C-5.

**Decision:** Adopt `025`'s directory layout and layering now. Treat the module list as open;
add `wallet`, `ratings`, `zones`, and `tracking` when their slices are built.

**Consequences:** Phase 1 creates no domain modules, so nothing is blocked. Revisit before the
first domain module lands.

**Affects:** `services/api/internal/` — future.

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
