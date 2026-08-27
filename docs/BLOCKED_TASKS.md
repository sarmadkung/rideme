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

## B-2 — Go ↔ TypeScript shared type strategy

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

**Relevant documents:** `023`, `025`, `193`, `331`, `014-api-specification`

**Decision required:** The source of truth for the API contract, and the generation direction.

**Options:**
- **Hand-maintained both sides** — simplest to start, drifts silently. Drift in a financial or
  dispatch payload is expensive.
- **Generate TS from Go** — single source of truth, needs tooling.
- **OpenAPI as contract, generate both** — strongest guarantee, most setup.

**Recommended:** Choose now, at the start of Phase 3, with an ADR. The cost of
deciding late is that every duplicated contract has to be migrated, and the
duplication only grows.

**Status:** OPEN — **due immediately.** Phase 1 shipped without it, as planned,
but three copies of one envelope already exist.

---

## B-3 — Business rules required before their domains are built

**Task:** Implement ride pricing, dispatch scoring, payments, settlement, and the service lifecycles.

**Reason:** 19 business rules are undocumented — cancellation fee amounts, dispatch scoring
weights, commission rates, refund policy, rounding rules, retention periods, and others.

**Relevant documents:** `BUSINESS_DECISION_REGISTER.md` (full catalogue with classifications)

**Decision required:** Product and commercial decisions from the owner. None can be safely inferred.

**Status:** OPEN — **does not block Phase 1 through Phase 3.** Each item is classified in the
register by when it actually becomes blocking. Six become blocking at the first vertical slice.
