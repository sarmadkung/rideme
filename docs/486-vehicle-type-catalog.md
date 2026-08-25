# 486 — Vehicle Type Catalog

## Objective
Define rickshaw, motorcycle, car, loader, van, pickup, truck, and future vehicle types with capabilities and restrictions.

## Position in the Blueprint
This is an execution-level specification for the logistics platform and must remain consistent with documents 191–484.

## Agent Instructions
Before implementation:
1. Read this document and its prerequisites.
2. Inspect the repository and identify existing implementations.
3. Reuse canonical models and shared abstractions.
4. Identify affected database, API, worker, realtime, ReactJS, and React Native components.
5. Implement incrementally with migrations where required.

## Core Requirements
- Maintain one authoritative source of truth for each business concept.
- Keep critical business rules server-side.
- Enforce authorization and validate all external input.
- Make critical mutations idempotent and concurrency-safe.
- Version externally consumed APIs/events when necessary.
- Protect personal, location, and financial data.
- Add observability for production-critical paths.

## Testing
Cover normal behavior, invalid state transitions, retries, duplicates, concurrency, dependency failures, and client synchronization where applicable.

## Acceptance Criteria
- The specified model/rules are implemented consistently.
- No competing source of truth is introduced.
- Existing functionality remains compatible or has an explicit migration.
- Critical invariants are enforced in code and/or database constraints.
- Automated tests pass.
- Security and observability requirements are satisfied.
- Typecheck, lint, build, and relevant tests pass.

## Handoff
Review the implementation, run verification, update the progress state from document 443, and continue to the next unblocked document.
