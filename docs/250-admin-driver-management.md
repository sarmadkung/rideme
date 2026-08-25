# 250 — Admin Driver Management

## Objective
Implement provider search, onboarding review, verification status, suspension, availability, vehicles, and operational history.

## Architectural Context
This is an implementation document in the logistics platform blueprint. Follow documents 191–244 and do not introduce a parallel architecture.

## Scope
Inspect the repository before implementation. Reuse existing modules, shared types, validation, API clients, UI components, domain services, and infrastructure where appropriate.

## Core Requirements
- Keep business-critical rules on the backend.
- Validate all external input.
- Enforce authorization server-side.
- Use explicit state transitions.
- Make critical mutations idempotent.
- Use versioned events for realtime/asynchronous behavior.
- Protect sensitive information.
- Add structured logging and correlation IDs to important operations.
- Add metrics for important success, failure, latency, and retry paths.
- Admin operations must use server-side authorization, auditable privileged actions, scoped data access, pagination, filtering, and safe mutation workflows.

## Implementation Tasks
1. Inspect current repository and relevant earlier documents.
2. Identify dependencies and prerequisites.
3. Define or reuse domain models.
4. Implement persistence and migrations where required.
5. Implement application/domain services.
6. Implement API contracts and validation.
7. Implement realtime/events/workers where required.
8. Implement the relevant web/mobile UI.
9. Add unit and integration tests.
10. Add critical end-to-end coverage.
11. Verify acceptance criteria.
12. Update implementation status and proceed only when prerequisites are complete.

## Failure & Recovery
Handle retries, duplicate requests, timeouts, stale state, disconnected clients, partial external failures, invalid state transitions, and operational escalation.

## Security
Use least privilege. Do not expose secrets or unnecessary personal/location/financial information. Privileged actions must be auditable.

## Data & API
Use the authoritative database and API architecture defined by earlier documents. Do not expose database models directly as public API contracts.

## Acceptance Criteria
- The specified workflow works end-to-end for its supported scope.
- Invalid operations are rejected.
- Authorization is enforced.
- Critical mutations are safe to retry.
- Failure and recovery paths are tested.
- Type checking and linting pass.
- Existing functionality remains intact.
- No duplicate implementation is introduced.
- Operationally important behavior is observable.

## Agent Handoff
After completing this document, run the relevant tests and checks, verify every acceptance criterion, record any architectural decision, and continue to the next incomplete document.
