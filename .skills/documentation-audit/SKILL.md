---
name: documentation-audit
description: Audits the 565-file docs/ set for conflicts, duplication, and empty boilerplate without reading it all. Use when running AGENTS.md Phase 0, when producing DOCUMENT_AUDIT.md or DOCUMENT_CONFLICTS.md, or whenever two documents appear to disagree about architecture, schema, or business rules.
---

# Purpose

Classify documentation cheaply and record disagreements instead of silently picking a side.

# When to Use

- AGENTS.md Phase 0.
- A doc contradicts another doc or the code.
- Deciding whether a doc is worth reading before a task.

# Rules

- **Never bulk-read.** Filenames plus one-line Objectives carry most of the signal. Full reads are for Tier A only (see `project-discovery`).
- Classify with grep and filename patterns, not with 500 file reads.
- Do not delete or renumber documents.
- Do not create missing numbered documents to satisfy a dangling reference — record the gap.
- When two docs conflict, apply the AGENTS.md §4 precedence: latest explicit architecture decision → master architecture → canonical domain model → dependency graph → README → existing validated implementation. If the tie survives that, it is unresolved, not yours to break.

# Known Findings (already established — do not re-derive)

1. **360 docs are template boilerplate** (`205`–`564`), in nine batches of exactly 40, each batch sharing one template. Unique content per file ≈ one Objective sentence. Full analysis in `docs/DOCUMENT_AUDIT.md` — do not re-derive it.
2. **Backend language conflict.** `docs/04` and `docs/12` lock the backend to **Go**; `AGENTS.md` Phase 1 prescribes a TypeScript-wide toolchain ("TypeScript configuration", `typecheck`). Go applies to `services/api/`, TS to `apps/` and `packages/`. Recorded, not silently resolved.
3. **Web framework drift.** `docs/04` says "Web: Next.js"; `docs/12` splits it — React+Vite for admin/merchant, Next.js for marketing only. `12` is the later, more specific "Locked Stack" and wins.
4. **Document 11 does not exist.** Numbering gap, not a missing dependency. **Document 564 does exist** — an earlier pass reported it missing; that was wrong.

5. **Conflicts are catalogued** in `docs/DOCUMENT_CONFLICTS.md` (C-1 … C-5) with resolutions in `docs/ARCHITECTURE_DECISIONS.md` (ADR-001 … ADR-005). Check there before re-investigating.

# Workflow

1. `ls docs/` for the filename inventory (cheap, high signal).
2. Grep for template markers to bucket Tier A/B/C.
3. For a specific question, read only the Tier A docs that own that topic.
4. Log conflicts to `docs/DOCUMENT_CONFLICTS.md` with both doc numbers, the disagreement, the precedence applied, and the outcome.
5. Update the counts in `docs/DOCUMENT_AUDIT.md`.

# Verification

Level 0. Documentation-only.

# Blocking Conditions

- Conflict survives the precedence rules → `docs/BLOCKED_TASKS.md`, with options and a recommendation. Do not choose.
- A doc asserts a business rule found nowhere else and nothing corroborates it → mark unresolved rather than treating it as a requirement.

# Relevant Documentation

`docs/365`–`368`, `443`, `444`, `564` (control docs, all Tier C) · `docs/10-decision-log.md` · `docs/369-architecture-decision-records.md`
