# 24 — Local Development Environment

## Objective

Make the entire platform reproducible on a developer machine.

## Required Local Services

```text
PostgreSQL + PostGIS
Redis
NATS
S3-compatible storage
```

Use Docker Compose for local infrastructure.

## Suggested Compose Services

```text
postgres
redis
nats
minio
```

The application itself can run directly on the host during development for faster iteration.

## Local Ports

Example convention:

```text
PostgreSQL  5432
Redis       6379
NATS        4222
NATS Monitor 8222
MinIO       9000
MinIO UI    9001
Go API      8080
```

Ports can be changed through environment variables.

## Database

Create a development database:

```text
logistics_dev
```

Enable PostGIS.

Run migrations automatically through an explicit command, not silently on every API startup.

## Seed Data

Provide deterministic seed data:

- admin user
- test customer
- test driver
- test merchant
- verified motorcycle
- verified car
- verified loader
- sample zones
- sample jobs

Never use real personal information in seed data.

## Environment Variables

Examples:

```text
APP_ENV=development
API_PORT=8080
DATABASE_URL=
REDIS_URL=
NATS_URL=
S3_ENDPOINT=
S3_BUCKET=
JWT_SECRET=
OTP_PROVIDER=
MAP_PROVIDER=
```

Secrets should be supplied locally and never committed.

## Developer Commands

Recommended commands:

```text
pnpm install
pnpm dev
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm e2e
```

Backend:

```text
go test ./...
go vet ./...
go run ./cmd/api
```

## Local Realtime Testing

Provide a development tool or admin endpoint that can simulate:
- driver online/offline
- driver movement
- job acceptance
- job completion

This allows dispatch testing without physically moving a phone.

## Local Failure Testing

Developers should be able to simulate:
- Redis unavailable
- NATS unavailable
- database unavailable
- WebSocket disconnect
- slow network
- stale GPS
- payment timeout

## Definition of Done

A new developer should be able to clone the repository and reach a working local environment using documented commands without manually installing infrastructure services.
