# RideMe

A multi-service logistics platform: rides, parcels, groceries, cargo and freight
over one dispatch and one fleet.

**Status: Phase 1 — foundation.** The repository runs end to end. It implements
no product functionality yet, and that is deliberate: see
[docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) for the sequence and
[docs/FIRST_IMPLEMENTATION_SLICE.md](docs/FIRST_IMPLEMENTATION_SLICE.md) for what
this slice does and does not contain.

## Prerequisites

| Tool    | Version | Notes                                        |
| ------- | ------- | -------------------------------------------- |
| Docker  | 24+     | Runs Postgres, Redis, NATS and MinIO locally |
| Node.js | 20+     |                                              |
| pnpm    | 10+     | `corepack enable`                            |
| Go      | 1.25+   | Version is pinned in `services/api/go.mod`   |

Nothing else needs installing by hand — no local Postgres, no local Redis.

## Getting started

```bash
git clone git@github.com:sarmadkung/rideme.git
cd rideme
make setup
```

`make setup` copies `.env.example` to `.env.local`, installs dependencies,
starts the infrastructure and applies migrations. Then, in two terminals:

```bash
make api-run    # Go API on :8080
pnpm dev        # admin dashboard and mobile shells
```

Check it worked:

```bash
curl -s localhost:8080/health | jq
```

Every dependency should report `healthy`. Stop a container and the same endpoint
answers `503` — a health check that cannot fail is decoration.

## Layout

```text
apps/
  customer-mobile/     Expo + React Native + TypeScript
  driver-mobile/       Expo + React Native + TypeScript
  admin-dashboard/     React + Vite + TypeScript
  merchant-dashboard/  reserved — scaffolded in its own slice
  marketing-web/       reserved — Next.js, the only place it is used
packages/              @platform/* shared TypeScript packages
services/api/          Go — independent module, its own toolchain (ADR-001)
infra/docker/          local infrastructure
docs/                  specification and control documents
.skills/               31 canonical project skills
.claude/skills         symlink to ../.skills for Claude Code
.agents/skills         symlink to ../.skills for Codex
```

The Go service is deliberately outside the pnpm workspace. Two toolchains, two
sets of commands, one repository — see
[ADR-001](docs/ARCHITECTURE_DECISIONS.md).

## Commands

`make help` lists everything. The ones that matter day to day:

| Command                                              | Does                                          |
| ---------------------------------------------------- | --------------------------------------------- |
| `make infra-up` / `infra-down`                       | Start / stop local infrastructure             |
| `make infra-reset`                                   | Destroy infrastructure **and all local data** |
| `make migrate-up` / `migrate-down`                   | Apply / roll back migrations                  |
| `make api-run`                                       | Run the Go API on the host                    |
| `make api-test` / `api-lint`                         | Go tests / `gofmt` + `go vet`                 |
| `pnpm dev` / `build` / `lint` / `typecheck` / `test` | Workspace tasks via Turborepo                 |
| `make verify`                                        | Every gate, both toolchains                   |

Migrations never run on API startup. They are an explicit command, so a schema
change cannot ride along with a deploy unnoticed.

## Environment

`.env.example` is the reference; copy it to `.env.local`, which is gitignored.

Variables split three ways, and the split is enforced rather than documented:
server-side secrets (`JWT_SECRET`, `DATABASE_URL`) are read only by the Go API;
`VITE_*` and `EXPO_PUBLIC_*` are compiled into client bundles and are therefore
public by definition. `@platform/config` rejects at startup any variable that
carries a public prefix and a secret-looking name.

## Ports

| Service              | Port        |
| -------------------- | ----------- |
| Go API               | 8080        |
| Admin dashboard      | 5173        |
| PostgreSQL + PostGIS | 5432        |
| Redis                | 6379        |
| NATS (monitoring)    | 4222 (8222) |
| MinIO (console)      | 9000 (9001) |

Override any of them in `.env.local`.

## Testing

```bash
pnpm test                                   # workspace: Vitest and jest-expo
make api-test                               # Go unit tests
cd services/api && go test -tags=integration ./tests/...   # needs infrastructure
```

Integration tests sit behind a build tag so `go test ./...` stays fast and needs
no running services. There are no E2E tests yet — there is no user journey to
exercise.

## Contributing

Branch from `main`, open a pull request, keep CI green. CI runs the JavaScript
and Go paths independently and only when their files change.

Before changing anything, read [AGENTS.md](AGENTS.md) — it is the execution
protocol this repository is built under — and the control documents in
[docs/](docs/): the implementation plan, current status, architecture decisions
and blocked tasks. Documents `00`–`190` are the authoritative specification.
