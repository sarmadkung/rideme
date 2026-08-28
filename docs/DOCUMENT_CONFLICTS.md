# Document Conflicts

Conflicts are recorded with the precedence applied (AGENT.md §4). Where precedence does not
settle a question, it is escalated to `BLOCKED_TASKS.md` rather than decided here.

---

## C-1 — Backend language vs. AGENT.md toolchain

**Documents:** `004`, `012`, `023`, `025` vs `AGENT.md` Phase 1

`004` and `012` lock the backend to **Go**. `023` and `025` elaborate it: Go lives at
`services/api/` as an independent application, explicitly *not* inside the JavaScript workspace.
`AGENT.md` Phase 1 prescribes a repository-wide TypeScript toolchain ("TypeScript configuration",
`typecheck`).

**Precedence:** master architecture (`012` Locked Stack) over the generic protocol document.

**Resolved:** Go for `services/api/` (`go build`, `go vet`, `go test`, `gofmt`). TypeScript for
`apps/` and `packages/` (pnpm, ESLint, Prettier, tsc). AGENT.md's "typecheck/lint/test/build"
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

**Not fully resolved.** Working position: `025`'s layout is authoritative for structure, with
`wallet`, `ratings`, and `zones` retained as modules when their slices are built. This does not
block Phase 1 — no module beyond the foundation is created in the first slice.

**Status:** DEFERRED — revisit when the first domain module is built. Recorded as ADR-004.

---

## C-6 — Roadmap conflicts (R-1 … R-8)

**Documents:** `AGENT.md` §9 · `IMPLEMENTATION_PLAN.md` · `MASTER_IMPLEMENTATION_ROADMAP.md`
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

## Non-conflicts (checked, not conflicting)

- **`012` vs `023` on Next.js** — consistent; both scope it to marketing.
- **`004` vs `013` on entities** — `013` implements `004`'s model; no disagreement.
- **`015` vs `197`** — `197` is a Tier B restatement of `015`; `015` has the detail.
- **`018` vs `200`** — `200` is Tier C; `018` holds the actual event list.
