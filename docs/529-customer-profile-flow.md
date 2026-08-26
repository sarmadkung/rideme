# 529 — Customer Profile Flow

## Objective
Implement customer profile creation, editing, verification state, preferences, and safe personal-data presentation.

## Position in the Blueprint
This is an execution-level workflow specification and must remain consistent with documents 191–524.

## Implementation Procedure
1. Read this document and prerequisites.
2. Inspect the current repository and identify existing implementation.
3. Trace the complete workflow across database, API, workers, realtime, web, mobile, and external providers.
4. Reuse canonical domain models and services.
5. Implement missing pieces incrementally.
6. Add tests before marking the workflow complete.
7. Verify failure, retry, authorization, and synchronization behavior.

## Required Behavior
- Backend remains authoritative for business rules.
- Validate every external input.
- Enforce authorization server-side.
- Critical mutations must be idempotent and concurrency-safe.
- Financial operations must use authoritative ledger/payment state.
- Location data must follow privacy and retention policies.
- Realtime clients must recover from missed events.
- Mobile clients must tolerate intermittent connectivity.
- External provider failures must produce controlled degraded behavior.

## Testing
Cover:
- successful workflow
- invalid input
- unauthorized access
- duplicate/retry behavior
- concurrent operations
- dependency failure
- timeout/network interruption
- stale client state
- recovery/resynchronization
- relevant end-to-end journey

## Acceptance Criteria
- Workflow works end-to-end.
- Every state transition is valid and authorized.
- API contracts are implemented consistently.
- Client state remains synchronized.
- Critical failures are handled.
- Tests pass.
- Observability exists.
- No duplicate source of truth is introduced.
- Progress state is updated.

## Agent Handoff
After verification, update document 443's progress state and select the next unblocked workflow from the dependency graph. Do not mark the workflow complete until the acceptance criteria are actually verified.
