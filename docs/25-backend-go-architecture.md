# 25 — Go Backend Architecture

## Objective

Create a modular Go backend that can start as a monolith and later split into services if required.

## Structure

```text
services/api/
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── identity/
│   ├── users/
│   ├── drivers/
│   ├── vehicles/
│   ├── documents/
│   ├── jobs/
│   ├── dispatch/
│   ├── pricing/
│   ├── tracking/
│   ├── payments/
│   ├── merchants/
│   ├── notifications/
│   ├── support/
│   ├── fraud/
│   └── analytics/
│
├── pkg/
│   ├── database/
│   ├── cache/
│   ├── messaging/
│   ├── storage/
│   ├── httpx/
│   ├── auth/
│   └── observability/
│
├── migrations/
├── tests/
└── go.mod
```

## Layering

Each domain should preferably separate:

```text
handler
  ↓
application/service
  ↓
domain
  ↓
repository / infrastructure
```

Do not make handlers contain business rules.

## Domain

Domain code contains:
- entities
- value objects
- state transitions
- business invariants

## Application Layer

Coordinates:
- use cases
- transactions
- authorization
- domain operations
- external dependencies

Example:

```text
CreateJob
QuoteJob
CancelJob
AcceptJob
CompleteJob
```

## Repository

Repositories abstract persistence.

Example:

```text
JobRepository
DriverRepository
VehicleRepository
PaymentRepository
```

Do not leak SQL details into domain code.

## HTTP

Handlers should:
1. authenticate
2. validate input
3. call application service
4. map result to HTTP response

## Errors

Use typed application errors:

```text
ErrNotFound
ErrUnauthorized
ErrForbidden
ErrConflict
ErrValidation
ErrUnavailable
```

Map them consistently to HTTP status codes.

## Transactions

Use explicit transactions around operations that must be atomic.

Example:

```text
Create job
+ create quote reference
+ create initial state
```

Payment and ledger operations must use strong transactional guarantees.

## Concurrency

Use database locks or transactional versioning where multiple workers may modify the same resource.

Critical examples:
- accepting the same job twice
- assigning the same driver twice
- spending the same wallet balance twice

## Configuration

Load configuration once at startup and validate required settings.

Do not access environment variables throughout business code.

## Observability

Every request should have:
- request ID
- structured logs
- latency
- status
- trace context

Critical workflows should emit domain metrics.

## Definition of Done

The backend foundation is ready when:
- server starts
- health checks work
- database connection works
- Redis connection works
- NATS connection works
- structured logging works
- tracing works
- error handling is consistent
- first domain module can be implemented without architectural changes
