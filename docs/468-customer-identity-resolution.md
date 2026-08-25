# 468 — Customer Identity Resolution

## Objective
Define safe handling of duplicate accounts, verified contact methods, device associations, and account merge/recovery workflows.

## Position in the Blueprint
This document is an execution-level specification in the logistics platform blueprint and must remain consistent with documents 191–444.

## Agent Instructions
Before implementation:
1. Read this document and all explicit prerequisites.
2. Inspect the repository and current implementation.
3. Reuse existing abstractions and authoritative models.
4. Identify migrations, compatibility risks, and affected applications.
5. Implement incrementally and verify each boundary.

## Requirements
- Keep authoritative business rules on the backend.
- Validate all external input and enforce authorization server-side.
- Preserve domain ownership and data authority.
- Make critical operations idempotent and concurrency-safe.
- Emit versioned events only where required.
- Protect sensitive data and secrets.
- Add structured observability.
- Add automated tests for normal, failure, retry, and concurrency paths.

## Acceptance Criteria
- The specified contract/model/rules are implemented and documented.
- Existing workflows remain compatible unless an explicit migration is provided.
- Critical invariants are enforced by code, constraints, or both.
- Tests cover important state transitions and failure cases.
- Security and authorization requirements are satisfied.
- Typecheck, lint, build, and relevant tests pass.
- No duplicate or conflicting source of truth is introduced.

## Handoff
After implementation, review the diff, run verification, update the machine-readable progress state from document 443, and continue only to the next unblocked document.
