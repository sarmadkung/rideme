# 359 — Customer Engagement

## Objective
Implement lifecycle notifications and campaigns with consent, frequency limits, personalization boundaries, and measurement.

## Context
This document extends the logistics platform implementation blueprint through document 364. It must follow the architecture, domain boundaries, API contracts, security model, and agent execution protocol already established.

## Implementation Principles
- Inspect the existing repository before making changes.
- Reuse existing abstractions and shared packages.
- Keep business-critical rules authoritative on the backend.
- Treat retries, duplicate events, disconnects, and partial failures as normal.
- Preserve backward compatibility unless a migration explicitly requires change.
- Add automated tests with implementation.
- Make operationally important behavior observable.
- Protect customer, provider, location, financial, and administrative data.
- Document any architectural decision that changes an earlier specification.

## Implementation Tasks
1. Read this document and all prerequisite documents.
2. Inspect existing code and identify what is already implemented.
3. Define dependencies and migration requirements.
4. Implement the required backend/domain behavior.
5. Implement web/mobile integration where applicable.
6. Implement realtime, background jobs, or infrastructure integration where applicable.
7. Add unit and integration tests.
8. Add end-to-end coverage for critical user journeys.
9. Add observability, security, and failure handling.
10. Verify every acceptance criterion.

## Failure & Recovery
Explicitly test and handle network interruption, duplicate requests/events, stale state, retries, timeouts, unavailable dependencies, invalid state, partial deployment, and rollback where applicable.

## Security
Apply least privilege, input validation, secure storage, safe logging, access controls, and auditability. Never expose secrets or unnecessary sensitive information.

## Performance
Define measurable expectations and test them where the feature can affect API latency, mobile performance, dispatch, realtime traffic, database load, or operational throughput.

## Acceptance Criteria
- The specified capability is implemented end-to-end for its supported scope.
- Existing workflows continue to work.
- Critical failure and recovery paths are tested.
- Security requirements are implemented.
- Observability is sufficient for production support.
- Type checking, linting, builds, and relevant tests pass.
- No duplicate or conflicting implementation is introduced.
- Documentation/status is updated.

## Agent Handoff
After completing this document, verify the acceptance criteria and run the relevant checks. If the implementation reveals a conflict with an earlier document, record a decision before proceeding.

## Definition of Done
A task is not complete merely because code exists. It is complete only when implementation, tests, integration, security, observability, and acceptance criteria are all satisfied.
