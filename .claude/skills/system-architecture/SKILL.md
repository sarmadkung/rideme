---
name: system-architecture
description: The platform's structural rules — modular monolith, layer boundaries, provider adapters, and where business logic is allowed to live. Use when creating a module, wiring an external service, deciding where code belongs, or when tempted to split out a microservice.
---

# Purpose

Keep the system a clean modular monolith so services can be extracted later, and keep business logic out of the places it leaks into.

# When to Use

- Adding a backend module or package.
- Integrating any external provider.
- Deciding which layer owns a piece of logic.
- Anyone proposes a new service or a shared "utils" grab-bag.

# Rules

- **Modular monolith first** (`docs/04`, `docs/12`). One Go backend in `services/api/`. Do not split into microservices until scale or team ownership justifies it — that split is an ADR, not a preference.
- **Domain logic stays independent** of HTTP handlers, database adapters, and external SDKs (`docs/09`). A domain service must be testable with no server and no network.
- **Every external provider sits behind an interface** (`docs/09`, `docs/391`):
  ```text
  Application → Provider Interface → Adapter → External Service
  ```
  Required for: maps, routing, payments, SMS, email, push, identity verification, storage. Provider SDK calls scattered through domain code are a defect.
- **Backend is authoritative.** Prices, permissions, and status transitions are never decided by a client.
- **One source of truth per concept.** If two modules both own "current job status", one of them is wrong.

# Module Shape

Backend modules (`docs/09`): identity, users, drivers, vehicles, documents, jobs, dispatch, pricing, payments, wallet, ratings, support, merchants, notifications, zones, fraud, analytics.

```text
services/api/<module>/
  domain/      entities, rules, state transitions — no I/O
  app/         application services, orchestration, transactions
  transport/   HTTP handlers, WebSocket handlers, DTO mapping
  store/       Postgres/Redis adapters
```

Dependencies point inward. `domain` imports nothing from `transport` or `store`.

# Workflow

1. Name the concept and find its owning module.
2. Check whether it already exists (`project-discovery`).
3. Place logic in the innermost layer that can hold it.
4. For an external service, define the interface first, then the adapter.
5. Confirm no second source of truth was created.

# Verification

Level 2 for module scaffolding; Level 3 once it exposes API or schema. New provider adapter: Level 3 plus an explicit failure-path test — providers fail, and degraded behavior must be controlled (`docs/392`).

# Blocking Conditions

- A change requires a microservice split → ADR and user decision.
- Two modules claim the same authoritative field → stop; resolve ownership via `docs/448` before coding.

# Relevant Documentation

`docs/04-domain-architecture.md` · `docs/09-project-structure.md` · `docs/12-technical-blueprint.md` · `docs/391-external-provider-adapters.md` · `docs/392-provider-fallbacks.md` · `docs/406-package-boundaries.md` · `docs/407-dependency-rules.md`
