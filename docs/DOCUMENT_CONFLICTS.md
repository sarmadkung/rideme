# Document Conflicts

Conflicts are recorded with the precedence applied (AGENTS.md §4). Where precedence does not
settle a question, it is escalated to `BLOCKED_TASKS.md` rather than decided here.

---

## C-1 — Backend language vs. AGENTS.md toolchain

**Documents:** `004`, `012`, `023`, `025` vs `AGENTS.md` Phase 1

`004` and `012` lock the backend to **Go**. `023` and `025` elaborate it: Go lives at
`services/api/` as an independent application, explicitly *not* inside the JavaScript workspace.
`AGENTS.md` Phase 1 prescribes a repository-wide TypeScript toolchain ("TypeScript configuration",
`typecheck`).

**Precedence:** master architecture (`012` Locked Stack) over the generic protocol document.

**Resolved:** Go for `services/api/` (`go build`, `go vet`, `go test`, `gofmt`). TypeScript for
`apps/` and `packages/` (pnpm, ESLint, Prettier, tsc). AGENTS.md's "typecheck/lint/test/build"
is read as *per-surface* verification, not one toolchain. **Recorded as ADR-001.**

**Status:** RESOLVED

---

## C-2 — Web framework scope

**Documents:** `004` vs `012`, `023`

`004` says "Web: Next.js". `012` splits it: React + Vite for merchant/admin dashboards, Next.js
for marketing only. `023` confirms `012`.

**Precedence:** later, more specific "Locked Stack" (`012`), corroborated by `023`.

**Resolved:** React + Vite for `merchant-dashboard` and `admin-dashboard`. Next.js **only** for
`marketing-web`. **Recorded as ADR-002.**

**Status:** RESOLVED

---

## C-3 — Application directory names

**Documents:** `009` vs `023`

`009`: `customer-mobile/`, `driver-mobile/`, `merchant-web/`, `admin-web/`, `marketing-web/`.
`023`: `customer-mobile/`, `driver-mobile/`, `merchant-dashboard/`, `admin-dashboard/`, `marketing-web/`.

**Precedence:** `023` is the dedicated repository-setup specification and is more specific.

**Resolved:** Use `023` names — `merchant-dashboard`, `admin-dashboard`. **Recorded as ADR-003.**

**Status:** RESOLVED

---

## C-4 — Shared package set

**Documents:** `009` vs `023`

`009` lists: ui, types, validation, maps, auth, config.
`023` lists the same plus **api-client**, and names them `@platform/*`.

**Precedence:** `023`, more specific and additive rather than contradictory.

**Resolved:** Seven packages under the `@platform/*` scope: ui, api-client, types, validation,
auth, maps, config.

**Status:** RESOLVED

---

## C-5 — Backend module list

**Documents:** `009` vs `025`

`009`: identity, users, drivers, vehicles, documents, jobs, dispatch, pricing, payments, **wallet**,
**ratings**, support, merchants, notifications, **zones**, fraud, analytics.

`025`: identity, users, drivers, vehicles, documents, jobs, dispatch, pricing, **tracking**,
payments, merchants, notifications, support, fraud, analytics.

`025` adds `tracking` and omits `wallet`, `ratings`, `zones`.

**Precedence:** `025` is the dedicated Go backend specification, but the omission looks like
abbreviation rather than a decision to drop those domains — `wallet` and `zones` have their own
Tier A documents (`053`, `097`) and `ratings` appears throughout `111`.

**Resolved 2026-08-28**, at the first domain module (roadmap Phase 4). The tie is broken by
`004`, which was not consulted when this conflict was recorded: its "Core Domains" list is
`009`'s exactly, including `wallet`, `ratings` and `zones`. Two Tier A documents against one,
and `004` is the master architecture.

The module list is the **union** — `004`/`009`'s seventeen plus `tracking` from `025`. `025`
remains authoritative for structure and layering, which was never in dispute. Modules are
created when their slice is built, not up front.

**Status:** RESOLVED — ADR-004, promoted from Proposed to Accepted.

---

## C-6 — Roadmap conflicts (R-1 … R-8)

**Documents:** `AGENTS.md` §9 · `IMPLEMENTATION_PLAN.md` · `MASTER_IMPLEMENTATION_ROADMAP.md`
· `BUSINESS_DECISION_REGISTER.md`

Creating `MASTER_IMPLEMENTATION_ROADMAP.md` (2026-08-27) surfaced eight items, catalogued in
full in that document's **Conflicts Discovered** section rather than duplicated here:

| ID | Item | Status |
|---|---|---|
| R-1 | Three incompatible phase numberings | RESOLVED by translation table |
| R-2 | No dedicated pricing/quote phase | **RESOLVED** (owner, 2026-08-28) — pricing is a shared capability, CAP-1, boundary created in Phase 7 |
| R-3 | Dispatch sequenced after ride booking | RESOLVED — ride E2E moved to the dispatch phase |
| R-4 | Location/realtime moved ahead of dispatch | RESOLVED in favour of the roadmap |
| R-5 | Horizontal client phases vs. the vertical slice rule | **RESOLVED** (owner, 2026-08-28) — vertical slices primary; shared client platform CAP-6; Phases 12/13 consolidate |
| C-7 | Verification state vocabularies (`016` vs `029`/`030`) | **RESOLVED** 2026-08-28 — dedicated implementation documents win |
| R-6 | Maps/routing/ETA, safety/fraud, notifications/chat/support, and analytics have **no phase** | **RESOLVED** (owner, 2026-08-28) — four cross-cutting tracks CAP-2…CAP-5 inside the existing 15 phases; B-4 closed |
| R-7 | Business decision register timeline uses the old numbering | RESOLVED by remapping |
| R-8 | BD-07 pulled from old Phase 3 to roadmap Phase 2 | RESOLVED — earlier is safer |

**Status:** **ALL RESOLVED** as of 2026-08-28. R-2, R-5 and R-6 were decided by the owner and
the resolutions are implemented in `MASTER_IMPLEMENTATION_ROADMAP.md`; B-4 is closed. No
architecture was changed silently.

**One material defect was found while resolving R-6 and is worth recording on its own:** the
roadmap had **no messaging capability before Phase 7**, yet `020` and `028` make phone OTP the
initial authentication method and require the OTP provider to sit behind an interface. Phase 3
authentication could not have shipped as previously scoped. Corrected — CAP-4's boundary is now
mandatory at Phase 3.

---

## C-7 — Verification state vocabularies

**Documents:** `016` vs `029`, `030`

`016` summarises verification as `PENDING_VERIFICATION -> VERIFIED | SUSPENDED | EXPIRED` for
vehicles and says nothing about driver verification states. `029` gives seven driver states
(`NOT_STARTED`, `IN_PROGRESS`, `SUBMITTED`, `UNDER_REVIEW`, `APPROVED`, `REJECTED`,
`SUSPENDED`); `030` gives six vehicle states, adding `UNDER_REVIEW` and `REJECTED` and naming
the first `PENDING`.

**Precedence:** `029` and `030` are the dedicated implementation documents and are more
specific. `016` is a summary state machine covering driver *availability*, where it remains
authoritative.

**Resolved 2026-08-28 (Phase 5):** driver verification follows `029`, vehicle verification
follows `030`, driver availability follows `016`. Migration `000004` widens the constraints
that `000003` had taken from `016`. Without `REJECTED` and a path back, `029`'s requirement
that "rejected documents can be resubmitted" would have been unrepresentable.

**Status:** RESOLVED

---

## C-8 — Job state names

**Documents:** `015` vs `036`

`015`'s main flow uses `ARRIVING` and `AT_DROPOFF`. `036`'s transition matrix uses
`ARRIVING_PICKUP` and `ARRIVING_DROPOFF` for the same two slots.

**Precedence:** `015` is the dedicated job state machine document and names the states;
`036` is the dedicated *rules* document and supplies the transition matrix, offer timeout,
reassignment, concurrency and cancellation behaviour, none of which `015` covers.

**Resolved 2026-08-28 (Phase 7):** state **names** follow `015` (already implemented in
migration `000003` and `internal/jobs`); transition **rules** follow `036`. The two documents
describe the same lifecycle and disagree only on two labels, so no behaviour is lost either
way — but two vocabularies in one codebase would be.

**Status:** RESOLVED

---

## Non-conflicts (checked, not conflicting)

- **`012` vs `023` on Next.js** — consistent; both scope it to marketing.
- **`004` vs `013` on entities** — `013` implements `004`'s model; no disagreement.
- **`015` vs `197`** — `197` is a Tier B restatement of `015`; `015` has the detail.
- **`018` vs `200`** — `200` is Tier C; `018` holds the actual event list.
