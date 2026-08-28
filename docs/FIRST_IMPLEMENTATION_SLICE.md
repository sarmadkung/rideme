# First Implementation Slice — Phase 1 Foundation

**Status:** SPECIFIED — not implemented.

## Objective

Produce a repository a developer can clone and run: workspace resolves, Go API starts, local
infrastructure comes up under Docker, quality gates execute, and CI is green — with **no product
functionality whatsoever**.

The measure of success is `023`'s and `025`'s own Definition of Done, not feature count.

## Scope

Grounded in `023` (monorepo), `024` (dev environment), `025` (Go backend), `012` (stack).

### 1. Workspace foundation
- pnpm workspaces + Turborepo at the root (`pnpm-workspace.yaml`, `turbo.json`, root `package.json`).
- TypeScript strict mode, shared `tsconfig` base.
- ESLint + Prettier, shared config.
- Directory skeleton per `023` and ADR-003.

### 2. Shared packages (`@platform/*`) — scaffolding only
`types`, `validation`, `api-client`, `auth`, `ui`, `maps`, `config`.
Each: builds, exports, is importable, has one passing placeholder test. **No domain types yet** —
those belong to Phase 5, and `@platform/types` filling up early is how a second source of truth starts.

### 3. Go backend foundation (`services/api/`)
Per `025`, independent of the JS workspace (ADR-001):
- `cmd/api/main.go`; `internal/` and `pkg/` directories created empty-but-present.
- `pkg/`: `database`, `cache`, `messaging`, `httpx`, `observability`, `config`.
- Configuration loaded and validated **once at startup** (`025`).
- Structured logging with request ID and trace context.
- Typed error taxonomy — `ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrConflict`,
  `ErrValidation`, `ErrUnavailable` — mapped consistently to HTTP status.
- Health checks proving Postgres, Redis, and NATS connectivity.
- Migration runner as an **explicit command**, never silently on startup (`024`).

### 4. Local infrastructure (`infra/docker/`)
Docker Compose per `024`: `postgres` (with PostGIS enabled), `redis`, `nats`, `minio`.
Ports per `024`'s convention. Development database `logistics_dev`.

### 5. Client application shells
- `apps/admin-dashboard` — React + Vite + TS. Boots, renders a placeholder, builds.
- `apps/customer-mobile`, `apps/driver-mobile` — Expo + RN + TS. Start in Expo, render a placeholder.
- `apps/merchant-dashboard`, `apps/marketing-web` — **directories only**; scaffolded in their own slices.

Shells prove the toolchain, nothing more. No navigation, no auth, no API calls.

### 6. Environment management
`.env.example` with the variable names from `024`. `.env.local` and `.env.test` gitignored.
`.gitignore` covering `.env*`, build output, `node_modules`, `.serena/`.

### 7. Testing infrastructure
A runner wired and executing for each surface — TS test runner for `apps/`+`packages/`, `go test`
for the backend. One meaningful test per surface. Not a test suite; a working harness.

### 8. CI foundation
GitHub Actions per `023`: `install → lint → typecheck → unit tests → build`, plus the Go path
(`gofmt` check, `go vet`, `go build`, `go test`). E2E gets its own workflow later, not this slice.

### 9. Documentation
Root `README.md` — currently absent — with clone-to-running instructions. Update
`verification-lite` with the real commands, and `project-discovery` with the new repository state.

---

## Explicitly Out of Scope

Listing these matters, because each is a plausible place to overreach:

- **Any domain model or business logic.** No User, Job, Vehicle, Assignment, Payment.
- **Any real API endpoint.** Health checks only.
- **Database schema beyond the migration mechanism.** No `013` tables yet — that is Phase 5.
- **Authentication or authorization.** Phase 4.
- **NATS event contracts.** Connectivity proven; no subjects defined.
- **WebSocket gateway.** Phase 3+.
- **UI beyond a placeholder screen.** No design system, no navigation, no component library.
- **`merchant-dashboard`, `marketing-web` scaffolding.**
- **Native modules.** No Swift, no Kotlin. `LocationService` arrives with the location slice.
- **Deployment, Terraform, cloud infrastructure.** Phase 15.
- **Seed data beyond what proves migrations run.** The full `024` seed set belongs with the domain.
- **E2E test infrastructure.**

## Dependencies

**External prerequisites:** Docker, Node + pnpm, Go toolchain on the developer machine.
No credentials, no external service accounts, no paid providers.

**Internal prerequisites:** none — this is the root of the dependency spine.

**Blocks:** everything. No other slice can start until this one is verified.

**Business decisions required:** none. Confirmed against `BUSINESS_DECISION_REGISTER.md`.

## Authoritative Documents

| Doc | Governs |
|---|---|
| `023-repository-monorepo-setup` | Workspace, package manager, app/package names, CI steps, Definition of Done |
| `024-development-environment` | Docker services, ports, env vars, database, developer commands |
| `025-backend-go-architecture` | Go layout, layering, errors, config, observability, Definition of Done |
| `012-technical-blueprint` | Locked stack |
| `009-project-structure` | Backend modules, provider-interface rule |
| `170-cicd-github-actions` | CI pipeline |
| `302-docker-and-local-infrastructure` | Local infrastructure (thin; `024` has the detail) |
| `177-testing-strategy-and-pyramid` | Testing approach |

ADRs applying: **ADR-001** (Go outside the JS workspace), **ADR-002** (Vite vs Next.js),
**ADR-003** (directory names).

## Expected Repository Structure

```text
rideme/
├── apps/
│   ├── customer-mobile/          Expo + RN + TS — shell
│   ├── driver-mobile/            Expo + RN + TS — shell
│   ├── admin-dashboard/          React + Vite + TS — shell
│   ├── merchant-dashboard/       (directory only)
│   └── marketing-web/            (directory only)
├── packages/
│   ├── types/  validation/  api-client/  auth/  ui/  maps/  config/
├── services/
│   └── api/                      Go — independent module
│       ├── cmd/api/main.go
│       ├── internal/             (empty, present)
│       ├── pkg/                  database cache messaging httpx auth observability
│       ├── migrations/
│       ├── tests/
│       └── go.mod
├── infra/
│   └── docker/docker-compose.yml postgres redis nats minio
├── .github/workflows/ci.yml
├── docs/                         (existing)
├── .skills/                      (31 canonical shared skills)
├── .claude/skills -> ../.skills  (Claude Code)
├── .agents/skills -> ../.skills  (Codex)
├── package.json  pnpm-workspace.yaml  turbo.json
├── .env.example  .gitignore
├── AGENTS.md
└── README.md
```

## Acceptance Criteria

From `023` and `025`'s own Definitions of Done:

1. `pnpm install` resolves the workspace with no errors.
2. All seven `@platform/*` packages build and are importable from an app.
3. `docker compose up` starts postgres (PostGIS enabled), redis, nats, minio.
4. The Go API starts and connects to Postgres, Redis, and NATS.
5. Health checks report each dependency accurately — and report *unhealthy* when a dependency is stopped.
6. Structured logging emits request ID and trace context.
7. The typed error taxonomy maps to HTTP status consistently.
8. Migrations run via an explicit command, and do **not** run on startup.
9. `admin-dashboard` builds and serves; both mobile shells start in Expo.
10. `pnpm lint`, `pnpm typecheck`, `pnpm test`, `pnpm build` all pass.
11. `gofmt -l` is clean; `go vet ./...`, `go build ./...`, `go test ./...` pass.
12. CI runs both paths and is green on a pull request.
13. No secrets are committed; `.env*` is ignored.
14. A new developer reaches a working environment from `README.md` alone, without manually
    installing infrastructure services.

## Minimal Verification

Per `verification-lite`. This slice is infrastructure, so it verifies itself by running — there is
no application behaviour to test.

| Item | Level | Verification |
|---|---|---|
| Workspace, configs, CI definition | 0–1 | Commands execute; CI green |
| Shared package scaffolding | 1 | Builds, imports resolve, placeholder tests pass |
| Client shells | 1 | Build and start |
| Go foundation, health checks, error mapping | 3 | `go test`, health endpoint checked both healthy **and** with a dependency stopped |
| Docker infrastructure | 5 | Infrastructure change: every service reachable; PostGIS extension confirmed present; migration up → down → up |

**Not required:** E2E of any kind. Load or performance testing. Security testing (no auth surface).
Cross-module integration tests (no modules).

**Evidence to report on completion:** the actual output of each command in criteria 10–12, the
health-check response in both states, and the migration up/down/up result. Per
`progress-tracking`, nothing is marked `VERIFIED` without that output.

## Follow-up Required In This Slice

`verification-lite` currently states that no application code exists and everything is Level 0.
**That must be corrected in the same slice**, with the real commands per surface. Leaving it stale
would cause silent under-verification from Phase 2 onward.
