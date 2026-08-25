# 199 — REST API Implementation

## Objective
Implement the versioned HTTP API using the contracts in document 193.

## Layers

```text
HTTP Route
 ↓
Validation
 ↓
Authorization
 ↓
Application Service
 ↓
Domain Logic
 ↓
Repository
 ↓
Database
```

## Rules
Controllers remain thin. Business logic belongs in application/domain services.

## Required middleware
- request ID
- authentication
- authorization
- validation
- error handling
- rate limiting where appropriate
- structured logging

## Errors
Use stable machine-readable error codes.

Examples:
```text
AUTH_REQUIRED
FORBIDDEN
VALIDATION_ERROR
RESOURCE_NOT_FOUND
INVALID_STATE
IDEMPOTENCY_CONFLICT
RATE_LIMITED
```

## Agent tasks
Implement MVP endpoints and shared API conventions.

## Acceptance criteria
API contracts, validation, authorization, errors, logs, and tests are implemented consistently.
