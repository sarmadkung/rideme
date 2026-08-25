# 194 — Monorepo Architecture

## Objective
Define the source repository structure and development conventions.

## Recommended stack
- pnpm workspaces
- TypeScript
- React Native
- ReactJS
- Node.js
- Docker
- GitHub Actions

## Structure

```text
apps/
  customer-mobile/
  driver-mobile/
  admin-web/

packages/
  ui/
  types/
  validation/
  api-client/
  config/
  domain/

backend/
  api/
  modules/
  workers/

infrastructure/
  aws/
  docker/
  github/

docs/
```

## Rules
- Shared code belongs in packages.
- Domain code should not be copied between applications.
- Applications should not directly access database code.
- Backend modules own their domain persistence and logic.
- Avoid circular package dependencies.
- Keep platform-specific mobile code isolated.

## Mobile
Customer and driver apps share:
- API client
- types
- validation
- common UI where appropriate
- domain utilities

They may differ in screens and workflows.

## Admin
Admin is a ReactJS application, separate from mobile applications but sharing types and API clients.

## Agent tasks
- Initialize workspace.
- Configure TypeScript.
- Configure linting/formatting.
- Configure testing.
- Establish dependency boundaries.
- Add CI.

## Acceptance criteria
A clean clone can install, type-check, lint, test, and build the repository.
