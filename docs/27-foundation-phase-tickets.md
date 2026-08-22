# 27 — Phase 1 Foundation Engineering Tickets

## Goal

Complete the engineering foundation before implementing the first customer/driver business workflows.

## Ticket FND-001 — Initialize Monorepo

### Tasks
- create pnpm workspace
- configure Turborepo
- create apps/packages directories
- enable TypeScript strict mode
- configure shared lint/format rules

### Acceptance
All workspace projects resolve and `pnpm install` succeeds.

---

## Ticket FND-002 — Customer Mobile Shell

### Tasks
- initialize Expo application
- Expo Router
- environment configuration
- TanStack Query
- Zustand
- error boundary
- basic navigation

### Acceptance
App launches on iOS and Android development builds.

---

## Ticket FND-003 — Driver Mobile Shell

Same foundation as customer mobile plus:
- location permission abstraction
- driver-specific navigation
- native development build configuration

### Acceptance
Driver app launches and native location capability can be integrated without architectural changes.

---

## Ticket FND-004 — Merchant Dashboard

### Tasks
- React + Vite
- routing
- authentication shell
- TanStack Query
- Zustand where needed
- shared UI package

### Acceptance
Authenticated dashboard shell renders successfully.

---

## Ticket FND-005 — Admin Dashboard

Same foundation as merchant dashboard, with role-aware routing.

### Acceptance
Admin routes are inaccessible to unauthorized roles.

---

## Ticket FND-006 — Shared Packages

Create:

```text
@platform/types
@platform/validation
@platform/api-client
@platform/auth
@platform/ui
@platform/maps
@platform/config
```

### Acceptance
At least one application successfully imports each applicable package.

---

## Ticket FND-007 — Go API Bootstrap

### Tasks
- Go module
- HTTP server
- configuration
- health endpoint
- readiness endpoint
- structured logging
- request ID
- graceful shutdown

### Acceptance

```text
GET /health
GET /ready
```

return correct status.

---

## Ticket FND-008 — Local Infrastructure

Create Docker Compose for:
- PostgreSQL + PostGIS
- Redis
- NATS
- MinIO

### Acceptance
A clean machine can start local infrastructure with one documented command.

---

## Ticket FND-009 — Database Migration System

### Tasks
- migration runner
- initial extensions
- migration folder
- development seed command

### Acceptance
Database can be created from zero using migrations only.

---

## Ticket FND-010 — CI Pipeline

Pull request pipeline:

```text
install
lint
typecheck
test
build
go test
go vet
```

### Acceptance
Broken lint/type/test/build blocks the PR.

---

## Ticket FND-011 — Observability Foundation

### Tasks
- Sentry
- OpenTelemetry
- structured logs
- trace/request correlation

### Acceptance
A test error and test request are visible with correlation information.

---

## Ticket FND-012 — Environment Documentation

Document:
- required variables
- local setup
- test setup
- service URLs
- development commands
- secrets policy

### Acceptance
New developer can follow README instructions and run the project.

---

## Phase 1 Exit Criteria

Do not start the Job/Dispatch implementation until:

- monorepo works
- all application shells work
- Go API works
- PostgreSQL/PostGIS works
- Redis works
- NATS works
- migrations work
- CI works
- observability works
- local setup is documented

At this point the project is ready for **Phase 2: Identity & Vehicles**.
