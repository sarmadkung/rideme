# Document Audit

**Date:** 2026-08-27 · **Method:** structural signature clustering (section-header fingerprint per file), not bulk reading.

## Summary

```text
Total files in docs/:          566   (564 numbered + README.md + generated control docs)
Numbered documents:            564   (000–564; 011 absent)
Missing numbered documents:      1   (011 only)

Authoritative (Tier A):        190   (000–190)
Needs review (Tier B):          14   (191–204)
Template/control (Tier C):     360   (205–564)
Duplicate:                       0   exact duplicates
Conflicting:                     5   see DOCUMENT_CONFLICTS.md
Obsolete:                        0   none superseded outright
Missing dependency:              0   all cross-references resolve
Implementation-ready:           ~40  Tier A docs with concrete, buildable specifications
```

## The Central Finding

Word count is a misleading measure of this document set:

| Tier | Docs | Words | Share of words | Share of information |
|---|---|---|---|---|
| A (000–190) | 190 | 28,321 | 20% | almost all |
| B (191–204) | 14 | 2,334 | 2% | thin restatements |
| C (205–564) | 360 | 109,600 | **78%** | ~one sentence each |

**Tier C is nine template batches of exactly 40 documents.** Each batch shares one section-header
signature and near-identical body text:

| Range | Docs | Template signature (abbreviated) |
|---|---|---|
| 205–244 | 40 | Objective · Scope · Requirements · Data & API · Failure Handling · Security · Observability |
| 245–284 | 40 | Objective · Architectural Context · Scope · Core Requirements · Data & API |
| 285–324 | 40 | Objective · Context · Implementation Principles · Failure Handling · Security |
| 325–364 | 40 | Objective · Context · Implementation Principles · Performance · Definition of Done |
| 365–404 | 40 | Objective · Context · Rules · Implementation Tasks · Failure Handling |
| 405–444 | 40 | Objective · Position in the Blueprint · Required Agent Behavior · Integration · Testing |
| 445–484 | 40 | Objective · Position in the Blueprint · Agent Instructions · Requirements |
| 485–524 | 40 | Objective · Position in the Blueprint · Agent Instructions · Core Requirements · Testing |
| 525–564 | 40 | Objective · Position in the Blueprint · Implementation Procedure · Required Behavior |

Within a batch, **the only content that varies is the title and a one-sentence Objective.** The
Rules, Implementation Tasks, Acceptance Criteria, and Handoff sections are copies.

### Consequence for implementation

Tier C documents are a **topic index**, not specifications. Their titles are genuinely useful —
they enumerate the platform's intended surface area, and several name real requirements
(`486-vehicle-type-catalog` names the seven vehicle types; `515-ledger-model` names double-entry).
But an agent that reads them expecting buildable detail will consume ~110,000 words and learn
almost nothing.

This is why `dependency-planning` derives implementation order from Tier A architecture rather
than from documents 366–368, and why `project-discovery` encodes the tier map.

## Tier A — Authoritative (000–190)

The substantive specifications. Highest-value documents identified so far:

| Doc | Content |
|---|---|
| `004` | Domain architecture, core entity model, Job abstraction, vehicle capability model |
| `005` | Dispatch filtering, scoring formula, pricing formulas, cancellation tiers |
| `009` | Repository structure, backend modules, provider-interface rule |
| `012` | **Locked stack** — the authoritative technology decision |
| `013` | Database schema, core tables and columns |
| `015` | Job state machine and `JobStatusChanged` |
| `016` | Driver and vehicle state machines, acceptance gates |
| `017` | React Native / native module boundary, `LocationService` interface |
| `018` | Realtime events, location pipeline, Redis/Postgres split |
| `019` | Payment flow, append-only ledger, COD, reconciliation |
| `023` | **Monorepo setup** — pnpm + Turborepo, app and package names, CI steps |
| `024` | **Local development environment** — Docker services, ports, seed data, commands |
| `025` | **Go backend architecture** — module layout, layering, errors, transactions |
| `026` | Database implementation |
| `177` | Testing strategy and pyramid |

## Tier B — Needs Review (191–204)

Fourteen thin documents (105–338 words) restating architecture at a high level. They contain
some unique content — `191` has a system diagram, `204` an execution protocol — but are largely
superseded in detail by Tier A. Read the Objective; do not expect implementation depth.

## Notes

- **No documents were deleted, renumbered, or created** to satisfy references.
- **Document 011 is absent** from the numbering. Nothing references it; this is a gap in the
  sequence, not a missing dependency.
- **All cross-references resolve.** The 228 document references in `.skills/` were
  validated against the filesystem.
- **Document 564 exists** (`564-agent-workflow-execution-spec.md`). An earlier audit pass
  reported it missing; that report was wrong and is corrected here.
