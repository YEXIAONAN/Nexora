SHELL := /bin/bash
.DEFAULT_GOAL := help

PSQL_ENV = set -a; [ -f .env ] && source .env; set +a;

.PHONY: help env dev-db dev-api dev-web migrate-up migrate-down migrate-status bootstrap \
	test test-api test-web lint lint-api lint-web build build-api build-web \
	compose-up compose-down compose-logs backup verify-backup secret-scan secret-scan-test api-check e2e verify \
	script-safety-test

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

env: ## Create .env from .env.example when missing
	@if [ ! -f .env ]; then cp .env.example .env && echo "created .env from .env.example"; else echo ".env already exists"; fi

dev-db: ## Start the PostgreSQL development container
	docker compose up -d postgres

dev-api: ## Run the Go API against the local database (requires .env)
	@test -f .env || (echo "missing .env; run 'make env'" && exit 1)
	$(PSQL_ENV) cd apps/api && go run ./cmd/server

dev-web: ## Run the Vite dev server
	cd apps/web && pnpm dev

migrate-up: ## Apply pending database migrations
	@test -f .env || (echo "missing .env; run 'make env'" && exit 1)
	$(PSQL_ENV) cd apps/api && go run ./cmd/migrate -command up

migrate-down: ## Roll back the latest migration (development only)
	@test -f .env || (echo "missing .env; run 'make env'" && exit 1)
	$(PSQL_ENV) cd apps/api && go run ./cmd/migrate -command down -steps 1

migrate-status: ## Print migration status
	@test -f .env || (echo "missing .env; run 'make env'" && exit 1)
	$(PSQL_ENV) cd apps/api && go run ./cmd/migrate -command status

bootstrap: ## Initialize the ROOT account interactively (no default password)
	@test -f .env || (echo "missing .env; run 'make env'" && exit 1)
	$(PSQL_ENV) cd apps/api && go run ./cmd/bootstrap

test: test-api test-web ## Run backend and frontend tests

test-api: ## Run Go unit tests
	cd apps/api && go test ./...

test-web: ## Run frontend unit tests
	cd apps/web && pnpm test

lint: lint-api lint-web ## Run all static checks

lint-api: ## Check Go formatting and vet
	@test -z "$$(gofmt -l apps/api/cmd apps/api/internal)" || (echo "gofmt required:"; gofmt -l apps/api/cmd apps/api/internal; exit 1)
	cd apps/api && go vet ./...

lint-web: ## Run ESLint, Prettier check and TypeScript check
	cd apps/web && pnpm lint && pnpm format:check && pnpm typecheck

build: build-api build-web ## Build backend binaries and the frontend bundle

build-api: ## Build Go binaries
	cd apps/api && go build ./...

build-web: ## Build the production frontend
	cd apps/web && pnpm build

compose-up: ## Build and start the full local stack (db, migrations, api, web)
	docker compose --profile full up -d --build

compose-down: ## Stop the local stack
	docker compose --profile full down

compose-logs: ## Follow logs of the full stack
	docker compose --profile full logs -f --tail=100

backup: ## Create and verify a database backup
	infra/scripts/backup.sh

verify-backup: ## Restore the newest backup into a scratch database
	infra/scripts/verify-backup.sh

secret-scan-test: ## Check scanner detection, exact exceptions and nonzero failures
	python3 -B -m unittest discover -s infra/scripts -p 'test_secret_scan.py'

secret-scan: secret-scan-test ## Scan tracked and nonignored new source without printing values
	infra/scripts/check-secrets.sh

script-safety-test: ## Prove the destructive restore drill refuses unsafe targets
	python3 -B -m unittest discover -s infra/scripts -p 'test_m6_split_restore_safety.py'

api-check: ## Check generated API types without writing source files
	cd apps/web && pnpm api:check

e2e: ## Run browser E2E on a disposable stack (devtest PG 5433, never the dev DB)
	bash infra/scripts/e2e.sh

verify: lint test build api-check secret-scan script-safety-test ## Run the full local verification gate
