# 443 — Agent Progress State

## Objective
Define a machine-readable implementation-status format so an autonomous coding agent can track completed, blocked, skipped, and verified documents.

## Position in the Blueprint
This is an execution-level specification for the logistics platform. Follow documents 191–404 and the established ReactJS dashboard + React Native mobile architecture.

## Required Agent Behavior
Before changing code:
1. Read this document and prerequisites.
2. Inspect the current repository.
3. Determine what is already implemented.
4. Reuse existing abstractions.
5. Identify migrations and compatibility risks.
6. Implement incrementally.

## Implementation Requirements
- Keep authoritative business rules server-side.
- Validate external input and enforce authorization server-side.
- Make critical mutations and asynchronous work retry-safe.
- Preserve supported API/event compatibility.
- Add tests with implementation.
- Add production observability.
- Protect sensitive data and secrets.

## Integration
Consider affected PostgreSQL/PostGIS, Redis, queues/workers, API, realtime, ReactJS, React Native, external providers, CI/CD, and infrastructure layers. Integrate only where required.

## Failure Handling
Handle duplicate requests/messages, retries, timeouts, stale state, connection loss, worker crashes, provider failures, partial deployment, and recovery.

## Testing
Use the appropriate combination of unit, integration, contract, realtime, web/mobile, end-to-end, and performance tests.

## Acceptance Criteria
- Capability follows established architecture.
- Existing behavior remains compatible unless an explicit migration exists.
- Critical paths have automated tests.
- Failure/retry behavior is verified.
- Security requirements are satisfied.
- Production observability exists.
- Typecheck, lint, build, and relevant tests pass.
- No duplicate/conflicting implementation exists.
- Implementation status is updated.

## Handoff
After completion, run verification, review the diff, confirm migrations/compatibility, update progress state, mark the document verified only when acceptance criteria are satisfied, and select the next unblocked document.
