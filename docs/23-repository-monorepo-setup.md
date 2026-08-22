# 23 — Repository & Monorepo Setup

## Objective

Create a production-ready monorepo for the logistics platform without over-engineering.

## Repository

```text
logistics-platform/
├── apps/
│   ├── customer-mobile/
│   ├── driver-mobile/
│   ├── merchant-dashboard/
│   ├── admin-dashboard/
│   └── marketing-web/
│
├── packages/
│   ├── ui/
│   ├── api-client/
│   ├── types/
│   ├── validation/
│   ├── auth/
│   ├── maps/
│   └── config/
│
├── services/
│   └── api/
│
├── infra/
│   ├── docker/
│   └── terraform/
│
├── docs/
├── .github/
│   └── workflows/
├── package.json
├── pnpm-workspace.yaml
├── turbo.json
└── README.md
```

## Package Manager

Use pnpm workspaces.

```text
packageManager: pnpm
```

Turborepo manages task orchestration and caching.

## Applications

### customer-mobile
React Native + Expo + TypeScript.

### driver-mobile
React Native + Expo + TypeScript.

### merchant-dashboard
React + Vite + TypeScript.

### admin-dashboard
React + Vite + TypeScript.

### marketing-web
Next.js + TypeScript.

## Shared Packages

### @platform/types
Shared domain types and enums.

### @platform/validation
Zod schemas shared across clients where appropriate.

### @platform/api-client
Typed API client.

### @platform/auth
Authentication abstractions.

### @platform/ui
Shared React UI/design-system components. Keep mobile and web-specific implementations where needed.

### @platform/maps
Map abstractions and shared geographic utilities.

### @platform/config
Shared non-secret configuration conventions.

## Backend

Go remains an independent application under:

```text
services/api/
```

Do not force Go into the JavaScript workspace.

## Environment Strategy

Files:

```text
.env.example
.env.local
.env.test
```

Never commit secrets.

Separate configuration by:
- development
- test
- staging
- production

## Code Quality

Use:
- ESLint
- Prettier
- TypeScript strict mode
- Go fmt
- Go vet
- automated tests

## Git

Recommended branches:

```text
main
develop
feature/*
fix/*
chore/*
```

Protect `main`.

All production changes go through pull requests.

## CI

Every pull request should run:

```text
install
lint
typecheck
unit tests
build
```

E2E tests can run in a dedicated workflow.

## Initial Definition of Done

The repository is ready when:
- all apps start locally
- shared packages resolve correctly
- Go API starts locally
- PostgreSQL/Redis/NATS can run through Docker
- CI passes
- environment configuration is documented
