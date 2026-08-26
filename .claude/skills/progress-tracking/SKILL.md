---
name: progress-tracking
description: Maintains honest implementation status in docs/IMPLEMENTATION_STATUS.md. Use after completing any implementation unit, before claiming a task is done, and whenever marking something VERIFIED — which requires evidence, not optimism.
---

# Purpose

Keep a status file that reflects reality, so the next session can trust it instead of re-auditing the repo.

# When to Use

- After every meaningful implementation unit.
- Before reporting completion to the user.
- When a task becomes blocked or turns out to be obsolete.

# Status Vocabulary

| Status | Means |
|---|---|
| `NOT_STARTED` | Identified, prerequisites unknown or unmet |
| `READY` | Prerequisites met, nothing written |
| `IN_PROGRESS` | Being worked now |
| `BLOCKED` | Cannot proceed; entry exists in `BLOCKED_TASKS.md` |
| `IMPLEMENTED` | Code exists and compiles; tests may be absent or failing |
| `VERIFIED` | Implemented **and** its `verification-lite` level passed, observed |
| `DEFERRED` | Deliberately postponed, with a reason |
| `OBSOLETE` | Superseded; the superseding item is named |

# Rules

- `IMPLEMENTED` ≠ `VERIFIED`. The gap between them is where false confidence lives. Code that compiles is `IMPLEMENTED`.
- `VERIFIED` requires observed output — the test counts, the passing line. If you cannot quote it, it is not verified.
- `BLOCKED` requires a matching `BLOCKED_TASKS.md` entry. A status with no explanation is noise.
- Downgrade freely. If a change broke something previously verified, mark it so.
- The file records reality, not intent. Never write an aspirational status.

# Format

```md
| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Auth registration | IMPLEMENTED | 12 pass | NO | E2E pending |
| Ride quote | READY | - | NO | needs pricing engine |
| Dispatch offer | BLOCKED | - | NO | see BLOCKED_TASKS #3 |
```

# Workflow

1. Finish the unit and its verification.
2. Choose the status the evidence supports — not the one you hoped for.
3. Update the row, including the actual test counts.
4. If blocked, write the `BLOCKED_TASKS.md` entry in the same edit.
5. Commit status alongside the code it describes, so they never drift.

# Verification

Level 0 — the status file is documentation. Its *content* must reflect verification that already happened.

# Blocking Conditions

- Cannot determine whether something is genuinely implemented → mark `NOT_STARTED` and note the uncertainty. Never guess upward.

# Relevant Documentation

`docs/443-agent-progress-state.md` · `docs/444-agent-final-execution-protocol.md` · `docs/564-agent-workflow-execution-spec.md` (all Tier C — intent only)
