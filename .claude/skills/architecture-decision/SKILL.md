---
name: architecture-decision
description: Records architectural decisions as ADRs when a choice departs from or resolves ambiguity in the documented architecture. Use when picking between viable designs, when a doc conflict needs settling, or when deviating from the locked stack — before writing the code, not after.
---

# Purpose

Make consequential choices visible and reversible, and stop the same debate recurring every session.

# When to Use

- Two viable designs and the docs do not choose.
- A change departs from the locked stack or an established boundary.
- A documentation conflict gets resolved by precedence.
- A new external provider, protocol, or data-authority boundary is introduced.

Not for routine choices — naming, file placement, obvious refactors. ADRs are for decisions someone will later ask "why?" about.

# Rules

- Record the decision **before** implementing it. An ADR written afterward is a rationalization.
- An ADR does not authorize inventing business requirements. If the missing piece is a product rule, that is a `BLOCKED_TASKS.md` entry, not an ADR.
- Never silently override the locked stack (`docs/12`). Deviating is an ADR with explicit reasoning.
- State what the decision costs, not only what it buys.

# Format

Append to `docs/ARCHITECTURE_DECISIONS.md`:

```md
## ADR-00N — <title>
**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Superseded by ADR-00M
**Context:** what forced a decision; which docs bear on it
**Decision:** what was chosen
**Alternatives:** what else was considered and why it lost
**Consequences:** what this makes easy, what it makes hard, what it forecloses
**Affects:** modules, contracts, migrations
```

# Workflow

1. State the decision in one sentence.
2. Check `docs/10-decision-log.md` and existing ADRs — it may already be settled.
3. Identify the governing docs (Tier A first).
4. Write the ADR with real alternatives, not strawmen.
5. Implement to match. If implementation reveals the ADR was wrong, supersede it — do not quietly diverge.

# Verification

Level 0 for the record itself. The implementation it governs carries its own level.

# Blocking Conditions

- The decision needs product input (pricing policy, cancellation fees, market rules) → `BLOCKED_TASKS.md`. These are the owner's calls.
- The decision would create a second source of truth for an existing concept → stop. That is a design error, not a tradeoff.

# Relevant Documentation

`docs/10-decision-log.md` · `docs/369-architecture-decision-records.md` · `docs/12-technical-blueprint.md` · `docs/448-ownership-and-data-authority.md`
