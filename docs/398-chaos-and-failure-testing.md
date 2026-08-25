# 398 — Chaos & Failure Testing

## Objective
Define controlled failure tests for databases, queues, Redis, external APIs, realtime systems, and mobile connectivity.

## Context
This document belongs to the implementation blueprint for the multi-service logistics platform. It must remain consistent with documents 191–364 and the established ReactJS dashboard + React Native mobile architecture.

## Rules
- Inspect the repository before implementation.
- Do not recreate an abstraction that already exists.
- Preserve established domain boundaries.
- Keep authoritative business state on the backend.
- Treat distributed failures, retries, duplicate messages, and disconnected clients as normal.
- Protect sensitive data.
- Add automated tests for important behavior and failure modes.
- Make production-critical behavior observable.
- Record architecture changes as ADRs.

## Implementation Tasks
1. Read prerequisite documents.
2. Inspect current implementation and determine what already exists.
3. Identify dependencies and migration requirements.
4. Implement the capability.
5. Integrate affected APIs, workers, database, realtime systems, web, and mobile clients.
6. Add tests.
7. Add observability.
8. Validate security.
9. Validate performance and operational impact.
10. Verify acceptance criteria.
11. Update implementation status.

## Failure Handling
Explicitly account for timeout, retry, duplicate execution, stale state, partial failure, dependency outage, deployment failure, and recovery.

## Acceptance Criteria
- The capability is implemented according to the architecture.
- Existing functionality remains compatible.
- Critical workflows have automated tests.
- Failure and recovery behavior is defined and tested.
- Security and authorization requirements are satisfied.
- Observability is sufficient for production operation.
- Build, typecheck, lint, and relevant tests pass.
- No duplicate or conflicting implementation exists.

## Agent Handoff
After completion, verify all acceptance criteria and prerequisites, record any architectural decisions, and proceed to the next incomplete document in the dependency-aware work queue.
