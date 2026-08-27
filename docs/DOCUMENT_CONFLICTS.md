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

## Non-conflicts (checked, not conflicting)

- **`012` vs `023` on Next.js** — consistent; both scope it to marketing.
- **`004` vs `013` on entities** — `013` implements `004`'s model; no disagreement.
- **`015` vs `197`** — `197` is a Tier B restatement of `015`; `015` has the detail.
- **`018` vs `200`** — `200` is Tier C; `018` holds the actual event list.
