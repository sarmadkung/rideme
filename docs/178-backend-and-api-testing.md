# 178 — Backend & API Testing

## Objective
Verify domain rules, APIs and service boundaries.

## Unit
Test:
- pricing
- eligibility
- state transitions
- commissions
- validation
- risk rules

## Integration
Test:
- PostgreSQL
- Redis
- queues
- object storage
- provider adapters

## API
Validate:
- authentication
- authorization
- schemas
- validation
- idempotency
- pagination
- error contracts

## Contract Testing
Important service boundaries should have contract tests to prevent accidental breaking changes.

## Definition of Done
Backend changes cannot silently break critical domain behavior or API contracts.
