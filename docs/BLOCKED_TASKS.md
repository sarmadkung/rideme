# Blocked Tasks

Items that cannot proceed without a decision the documentation does not make. Business-rule gaps
are catalogued in `BUSINESS_DECISION_REGISTER.md`; this file holds items that **block work now or
imminently**.

**Nothing blocks Phase 1.** The foundation slice is fully specified by documents `023`, `024`,
`025`, `012`, and `013`.

---

## B-1 — Control documents 366/367/368 contain no control data

**Task:** Follow the AGENT.md execution protocol as written.

**Reason:** `366-implementation-dependency-graph`, `367-implementation-phases`, and
`368-agent-work-queue` are Tier C template documents. `366` contains no graph, `367` no phase
list, `368` no queue. The protocol's machine-readable substrate does not exist.

**Relevant documents:** `366`, `367`, `368`, `443`, `444`, `564`, `DOCUMENT_AUDIT.md`

**Decision required:** Whether to (a) proceed with the derived dependency spine in
`IMPLEMENTATION_PLAN.md`, or (b) author real content for `366`–`368` first.

**Options:**
- **(a) Proceed with the derived spine.** Order comes from Tier A architecture. Already
  implemented in `dependency-planning`. No work stalls.
- **(b) Author 366–368 properly.** Produces the documented artifacts, but is specification
  authoring — a task for the owner, and it duplicates what the spine already encodes.
- **(c) Both** — proceed now, backfill the documents later from what the spine proves out.

**Recommended:** **(c)**. Proceeding is not blocked; ADR-005 records the bypass. Backfilling
later costs nothing and is more accurate once real dependencies are observed.

**Status:** OPEN — does not block Phase 1.

---

## B-2 — Go ↔ TypeScript shared type strategy · **CLOSED**

**Task:** Share domain types between the Go backend and TypeScript clients.

**Reason:** `023` specifies `@platform/types` for shared domain types and `@platform/validation`
for Zod schemas. `025` specifies Go domain entities. Neither document says how the two stay in
sync — hand-maintained, generated from Go, or generated from a schema (OpenAPI/protobuf).
`193-api-contracts` and `331-api-contract-testing` are Tier C and add nothing.

**Now concrete.** Phase 1 already duplicated one contract by hand: the error
taxonomy exists in `services/api/pkg/httpx/errors.go` and again in
`packages/types/src/errors.ts`, with the health envelope duplicated alongside it
and re-expressed a third time as Zod in `packages/validation`. Three hand-kept
copies of one contract, today, with no domain payloads yet. Each is marked with a
pointer back to this item.

**Relevant documents:** `023`, `025`, `193`, `331`, `14-api-specification`

**Decision required:** The source of truth for the API contract, and the generation direction.

**Options:**
- **Hand-maintained both sides** — simplest to start, drifts silently. Drift in a financial or
  dispatch payload is expensive.
- **Generate TS from Go** — single source of truth, needs tooling.
- **OpenAPI as contract, generate both** — strongest guarantee, most setup.

**Recommended:** Choose now, at the start of Phase 3, with an ADR. The cost of
deciding late is that every duplicated contract has to be migrated, and the
duplication only grows.

**Resolution (2026-08-28, Phase 2):** **Go is the single source of truth.**
`packages/types/src/generated.ts` and `packages/validation/src/generated.ts` are
generated from the Go types by `services/api/cmd/contractgen`, which reflects
over the registered structs. `make contracts` regenerates; `make contracts-check`
fails on stale output and runs inside `make verify`. Hand-written TypeScript may
no longer declare a wire shape — the three duplicated copies were deleted and
replaced by generated output. Recorded as **ADR-007**.

**Status:** **CLOSED — RESOLVED 2026-08-28.** Blocks nothing.

---

## B-3 — Business rules required before their domains are built

**Task:** Implement ride pricing, dispatch scoring, payments, settlement, and the service lifecycles.

**Reason:** 19 business rules are undocumented — cancellation fee amounts, dispatch scoring
weights, commission rates, refund policy, rounding rules, retention periods, and others.

**Relevant documents:** `BUSINESS_DECISION_REGISTER.md` (full catalogue with classifications)

**Decision required:** Product and commercial decisions from the owner. None can be safely inferred.

**Status:** OPEN — **does not block Phase 1 through Phase 3.** Each item is classified in the
register by when it actually becomes blocking. Six become blocking at the first vertical slice.

---

## B-4 — Four documented domains have no phase in the master roadmap · **CLOSED**

**Task:** Build maps/routing/ETA, safety/trust/fraud, notifications/chat/support, and analytics.

**Reason:** The 15-phase master roadmap covers none of them, yet each owns a substantial Tier A
band:

| Domain | Tier A documents | Where it bites |
|---|---|---|
| Maps, routing, ETA | `93`–`106` | **Most serious.** ETA is a term in the `005` dispatch scoring formula and in customer tracking — Phases 6, 7 and 8 all need it. |
| Safety, trust, fraud | `107`–`120` | SOS, trip sharing, ratings, fraud engine, enforcement. |
| Notifications, chat, support | `121`–`134` | Phases 7, 10 and 12 name "notifications" with no phase that builds the notification system. |
| Analytics and BI | `149`–`162` | No phase at all. |

**Decision required:** Whether these become Phases 16+, are folded into existing phases as
scope, or are explicitly out of the initial launch.

**Interim position (does not block Phase 2):** build the minimum each phase genuinely requires
and no more — a routing/ETA provider abstraction in Phase 6 (`95`, `96`), a notification
dispatch interface where Phases 7 and 10 need one — and record each as an explicit partial.
Do not build these domains out speculatively.

**Recommended:** fold maps/routing/ETA into Phase 6 as named scope (it is a hard dependency of
Phases 7 and 8); decide the other three separately, as they are closer to launch scope than to
dependency.

**Resolution (owner decision, 2026-08-28):** resolved as **four cross-cutting capability
tracks inside the existing 15 phases** — no new phases, no renumbering. CAP-2 maps/routing/ETA
(boundary Phase 6), CAP-3 safety/trust/fraud (Phase 3 device trust · Phase 5 verification ·
Phase 8 ratings · Phase 11 fraud engine), CAP-4 notifications/chat/support (**boundary Phase 3,
mandatory**), CAP-5 analytics (envelope Phase 2, pipeline Phase 14). Staged in full in
`MASTER_IMPLEMENTATION_ROADMAP.md` → Cross-Cutting Capabilities.

Dependency analysis also corrected a real sequencing defect: `020` and `028` make phone OTP the
initial authentication method and require the OTP provider to sit behind an interface, so a
messaging capability is required at **Phase 3**, not Phase 7. Phase 3 scope and acceptance were
updated accordingly.

**Status:** **CLOSED — RESOLVED 2026-08-28.** Blocks nothing.


---

## B-5 — BD-04: what happens when dispatch finds nobody · **OPEN**

**Task:** Terminate a job that no driver will take.

**Reason:** The dispatch engine is built and bounded — document 044 requires retries be finite,
and they are: configurable rings, a configurable attempt cap, and `ErrNoSupply` when they are
exhausted. What is *not* decided is what the customer sees at that point.

Document 015 gives `EXPIRED` as a terminal state, so the shape exists. Document 044 says to
"keep job searching where appropriate, notify customer, provide cancellation option, escalate
to operations for exceptional cargo" — four behaviours, with no rule for choosing between them
and no durations attached to any of them.

**What was built anyway.** The engine stops after the configured attempts and reports
`ErrNoSupply`. It deliberately does **not** expire the job: choosing that timeout would be
inventing the customer-facing rule. Everything up to the decision point works and is tested;
only the decision is absent.

**Decision required:**

- How long should a job keep searching before it stops?
- On exhaustion: expire the job, keep searching at a slower cadence, or escalate to an operator?
- What does the customer see while searching, and when it stops?
- Does the answer differ by service — a cargo job may warrant escalation where a ride does not?

**Recommendation:** the search window and retry cadence can be configuration with sensible
starting values. The customer-facing behaviour cannot, and is the actual question.

**Status:** OPEN — **does not block Phases 9–11.** It blocks going live with dispatch.
