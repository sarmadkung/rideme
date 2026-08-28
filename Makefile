# Developer entry points. `make help` lists everything.
#
# The JavaScript workspace and the Go service are separate toolchains by
# design (ADR-001); these targets are the seam between them.

# Compose reads .env from its own directory, not the repository root, so the
# root .env.local is passed explicitly. It is optional: a fresh clone can start
# the infrastructure on the documented defaults before running `make setup`.
ENV_FILE := $(wildcard .env.local)
COMPOSE  := docker compose $(if $(ENV_FILE),--env-file $(ENV_FILE)) -f infra/docker/docker-compose.yml
API     := services/api

.DEFAULT_GOAL := help
.PHONY: help setup infra-up infra-down infra-logs infra-reset \
        migrate-up migrate-down migrate-version api-run api-test api-lint \
        install dev lint typecheck test build verify

help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: ## First-time setup: env file, dependencies, infrastructure, migrations
	@test -f .env.local || (cp .env.example .env.local && echo "created .env.local")
	pnpm install
	cd $(API) && go mod download
	$(MAKE) infra-up
	$(MAKE) migrate-up
	@echo "Ready. Run 'make api-run' and 'pnpm dev'."

# --- local infrastructure ----------------------------------------------------

infra-up: ## Start postgres, redis, nats and minio
	$(COMPOSE) up -d --wait

infra-down: ## Stop infrastructure, keeping data
	$(COMPOSE) down

infra-logs: ## Follow infrastructure logs
	$(COMPOSE) logs -f

infra-reset: ## Destroy infrastructure and all local data
	$(COMPOSE) down -v

# --- database ----------------------------------------------------------------

migrate-up: ## Apply pending migrations
	cd $(API) && go run ./cmd/migrate up

migrate-down: ## Roll back every migration (destructive)
	cd $(API) && go run ./cmd/migrate down

migrate-version: ## Print the applied schema version
	cd $(API) && go run ./cmd/migrate version

# --- backend -----------------------------------------------------------------

api-run: ## Run the Go API on the host
	cd $(API) && go run ./cmd/api

api-test: ## Run Go unit tests
	cd $(API) && go test ./...

api-lint: ## gofmt and go vet
	@cd $(API) && test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)
	cd $(API) && go vet ./...

# --- contracts ---------------------------------------------------------------

contracts: ## Regenerate the TypeScript contract from the Go types (ADR-007)
	cd $(API) && go run ./cmd/contractgen -root ../..
	pnpm exec prettier --write packages/types/src/generated.ts packages/validation/src/generated.ts

contracts-check: contracts ## Fail if the checked-in contract is stale
	@git diff --quiet -- packages/types/src/generated.ts packages/validation/src/generated.ts \
		|| (echo "" && \
		    echo "The generated contract is out of date." && \
		    echo "A Go type changed without 'make contracts' being run." && \
		    echo "The regenerated files are in your working tree; commit them." && \
		    exit 1)

# --- javascript workspace ----------------------------------------------------

install: ## Install workspace dependencies
	pnpm install

dev: ## Start the workspace in development mode
	pnpm dev

lint: ## Lint the workspace
	pnpm lint

typecheck: ## Typecheck the workspace
	pnpm typecheck

test: ## Run workspace tests
	pnpm test

build: ## Build the workspace
	pnpm build

# --- everything --------------------------------------------------------------

verify: api-lint api-test contracts-check lint typecheck test build ## Run every quality gate, both toolchains
