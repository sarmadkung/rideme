# 215 — Driver Vehicle Assignment

## Objective
Implement provider-to-vehicle assignment history and active operating vehicle rules.

## Scope
This document is an implementation specification. Inspect the existing repository before adding code and reuse existing abstractions where possible.

## Requirements
- Follow documents 191–204 and the latest approved architecture.
- Keep business-critical rules on the server.
- Validate all external input.
- Enforce authorization server-side.
- Make critical mutations idempotent.
- Emit and consume versioned events where realtime/asynchronous behavior is required.
- Protect sensitive information.
- Add observability for production-critical operations.
- Design for unreliable mobile connectivity where applicable.

## Implementation Tasks
1. Inspect existing modules, database models, APIs, packages, and tests.
2. Identify reusable code before creating new abstractions.
3. Implement the domain model and persistence required by this document.
4. Implement API/application services and validation.
5. Implement realtime/events/workers required by the workflow.
6. Implement client integration where this document concerns mobile applications.
7. Add unit, integration, and critical end-to-end tests.
8. Update relevant documentation/status without duplicating README files.

## Data & API
Use the schemas and contracts defined in documents 192, 193, 195, and 199. Any new table or endpoint must have an explicit owner and documented lifecycle.

## Failure Handling
Handle retries, duplicate requests, timeouts, disconnected clients, stale state, invalid state transitions, and partial external-provider failures.

## Security
Apply least privilege, avoid leaking sensitive data, validate authorization for every protected operation, and never place secrets in client code.

## Observability
Use request IDs/correlation IDs and structured logs. Add metrics for important success, failure, latency, and retry paths.

## Acceptance Criteria
- The specified workflow is implemented end-to-end where applicable.
- Invalid state transitions are rejected.
- Authorization is enforced.
- Critical mutations are safe to retry.
- Automated tests cover normal and failure paths.
- Type checking and linting pass.
- No duplicate implementation is introduced.
- Existing functionality remains intact.

## Agent Handoff
After completing this document, verify its acceptance criteria, run the relevant test suite, update implementation status, and proceed to the next incomplete document only if prerequisites are satisfied.
