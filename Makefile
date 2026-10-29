SHELL := /bin/bash
.DEFAULT_GOAL := help

PSQL_ENV = set -a; [ -f .env ] && source .env; set +a;

.PHONY: help env dev-db dev-api dev-web migrate-up migrate-down migrate-status \
	test test-api test-web lint lint-api lint-web build build-api build-web \
	compose-up compose-down compose-logs backup verify-backup secret-scan verify

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

secret-scan: ## Scan tracked files for common secret patterns
	infra/scripts/check-secrets.sh

verify: lint test build secret-scan ## Run the full local verification gate
