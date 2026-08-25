# 203 — Monorepo Tooling & Development Environment

## Objective
Make local development reproducible.

## Required tooling
- pnpm
- TypeScript
- ESLint
- Prettier
- test runner
- Git hooks where useful
- environment validation
- Docker Compose for local dependencies

## Commands
Define consistent commands for:
```text
install
dev
build
lint
typecheck
test
test:e2e
db:migrate
db:seed
```

## Environment
Validate required environment variables at startup.

Never commit secrets.

## Agent tasks
- Configure workspace.
- Configure shared TS settings.
- Configure lint/format.
- Configure local PostgreSQL/Redis.
- Document setup.

## Acceptance criteria
A new developer can run the platform locally using documented commands.
