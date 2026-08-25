# 193 — API Contracts

## Objective
Define stable contracts between applications and backend domains.

## API style
Use REST for ordinary application commands/queries unless another protocol has a clear advantage.

Use:
- WebSocket/realtime for live state
- gRPC for internal high-throughput service communication when justified
- GraphQL only where flexible aggregation materially benefits clients

## API conventions

```text
/api/v1/...
```

Every response should have consistent:
- status
- data
- error
- request/correlation identifier

## Core endpoints

### Identity
```http
POST /auth/register
POST /auth/login
POST /auth/refresh
POST /auth/logout
```

### Vehicles
```http
POST /vehicles
GET /vehicles
GET /vehicles/:id
PATCH /vehicles/:id
POST /vehicles/:id/documents
```

### Services
```http
GET /services
GET /services/:id
```

### Quotes
```http
POST /quotes
GET /quotes/:id
```

### Bookings
```http
POST /bookings
GET /bookings/:id
POST /bookings/:id/cancel
```

### Jobs
```http
POST /jobs/:id/accept
POST /jobs/:id/reject
POST /jobs/:id/start
POST /jobs/:id/complete
```

### Tracking
```http
POST /tracking/sessions
POST /tracking/location
GET /tracking/:id
```

### Payments
```http
POST /payments/intents
GET /payments/:id
POST /payments/:id/refund
```

## Rules
- Validate all inputs server-side.
- Use idempotency keys for critical mutations.
- Never trust client pricing.
- Never expose internal database models directly.
- Version breaking API changes.
- Return actionable error codes.

## Agent tasks
Create typed request/response schemas and shared client types.

## Acceptance criteria
All MVP workflows have documented request, response, validation, authorization, and error contracts.
