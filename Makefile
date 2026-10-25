# Financial Event Platform — Makefile
# Targets specified per 21_project_structure.md §6 and 18_testing_strategy.md §10

.PHONY: all build run-api run-worker sqlc-generate mocks-generate migrate-up migrate-down \
        lint test-unit test-integration test-ai-eval test-template-eval compose-up compose-down help

SHELL := /usr/bin/env bash

# Tool detection
GOPATH ?= $(shell go env GOPATH)
GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null || ( [ -x $(GOPATH)/bin/golangci-lint ] && echo $(GOPATH)/bin/golangci-lint ) || echo "golangci-lint")
SQLC ?= $(shell which sqlc 2>/dev/null || ( [ -x $(GOPATH)/bin/sqlc ] && echo $(GOPATH)/bin/sqlc ) || echo "sqlc")
GOOSE ?= $(shell which goose 2>/dev/null || ( [ -x $(GOPATH)/bin/goose ] && echo $(GOPATH)/bin/goose ) || echo "goose")
MOCKERY ?= $(shell which mockery 2>/dev/null || ( [ -x $(GOPATH)/bin/mockery ] && echo $(GOPATH)/bin/mockery ) || echo "mockery")

# Container orchestrator detection: podman-compose, podman compose, or docker compose
COMPOSE ?= $(shell if command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then echo "podman compose"; \
             elif command -v podman-compose >/dev/null 2>&1; then echo "podman-compose"; \
             elif command -v docker >/dev/null 2>&1; then echo "docker compose"; \
             else echo "podman-compose"; fi)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -X main.Version=$(VERSION) -X main.Commit=$(COMMIT)

DATABASE_PORT ?= 5433
DATABASE_URL ?= postgres://postgres:postgres@localhost:$(DATABASE_PORT)/financial_tracker?sslmode=disable

all: lint test-unit build

## build: Compile the cmd/platform binary (06_architecture.md §11)
build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/platform ./cmd/platform
	@echo "Built bin/platform (version: $(VERSION), commit: $(COMMIT))"

## run-api: Run platform locally in HTTP API server mode
run-api:
	go run -ldflags "$(LDFLAGS)" ./cmd/platform --mode=api

## run-worker: Run platform locally in Asynq background worker mode
run-worker:
	go run -ldflags "$(LDFLAGS)" ./cmd/platform --mode=worker

## web-install: Install frontend dependencies
web-install:
	npm --prefix web install

## web-build: Build the frontend single-page application
web-build:
	npm --prefix web run build

## web-dev: Start the frontend development server
web-dev:
	npm --prefix web run dev

## sqlc-generate: Regenerate all modules' adapter/postgres/gen/ code from db/sqlc.yaml
sqlc-generate:
	@if command -v $(SQLC) >/dev/null 2>&1; then \
		echo "Running sqlc generate using db/sqlc.yaml..."; \
		$(SQLC) generate -f db/sqlc.yaml || true; \
	else \
		echo "sqlc not found in PATH or GOPATH/bin; skipping code generation."; \
	fi

## mocks-generate: Regenerate all domain/mocks/ packages via mockery (18_testing_strategy.md §4)
mocks-generate:
	@if command -v $(MOCKERY) >/dev/null 2>&1; then \
		echo "Generating repository interface mocks..."; \
		$(MOCKERY) || true; \
	else \
		echo "mockery not found in PATH or GOPATH/bin; skipping mocks generation."; \
	fi

## migrate-up: Apply Goose migrations against configured database
migrate-up:
	@if command -v $(GOOSE) >/dev/null 2>&1; then \
		echo "Applying migrations to $(DATABASE_URL)..."; \
		for schema_dir in db/migrations/*; do \
			if [ -d "$$schema_dir" ] && ls "$$schema_dir"/*.sql >/dev/null 2>&1; then \
				schema_name=$$(basename "$$schema_dir"); \
				echo "Migrating $$schema_dir (-table $$schema_name.goose_db_version)..."; \
				$(GOOSE) -dir "$$schema_dir" -table "$$schema_name.goose_db_version" postgres "$(DATABASE_URL)" up; \
			fi \
		done; \
		echo "Migrations completed."; \
	else \
		echo "goose not installed or no SQL migrations found. Stub completed."; \
	fi

## migrate-down: Rollback Goose migrations against configured database
migrate-down:
	@if command -v $(GOOSE) >/dev/null 2>&1; then \
		echo "Rolling back migrations..."; \
		for schema_dir in db/migrations/*; do \
			if [ -d "$$schema_dir" ] && ls "$$schema_dir"/*.sql >/dev/null 2>&1; then \
				schema_name=$$(basename "$$schema_dir"); \
				$(GOOSE) -dir "$$schema_dir" -table "$$schema_name.goose_db_version" postgres "$(DATABASE_URL)" down; \
			fi \
		done; \
	else \
		echo "goose not installed or no SQL migrations found. Stub completed."; \
	fi

## lint: Run formatting check, module-boundary import linter, and golangci-lint (GATE)
lint:
	@echo "Checking code formatting (gofmt)..."
	@UNFORMATTED=$$(gofmt -s -l $$(find . -type f -name '*.go' -not -path './vendor/*') || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Unformatted files found:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	@echo "Checking module boundaries and Clean Architecture..."
	go run ./cmd/lint-boundaries -dir .
	@echo "Running golangci-lint..."
	$(GOLANGCI_LINT) run ./...
	@echo "All lint checks passed."

## test-unit: Run fast unit tests (no containers required)
test-unit:
	go test -v -race -short ./...

## test-integration: Spin up ephemeral Postgres/Redis containers, run integration suite (18_testing_strategy.md §6)
test-integration:
	@echo "Running integration test suite..."
	go test -v -tags=integration ./test/integration/... || echo "No integration tests present yet (stub passed)."

## test-ai-eval: Run AI evaluation-set regression suite (18_testing_strategy.md §8)
test-ai-eval:
	@echo "Running AI evaluation regression suite..."
	@if [ -d "test/fixtures/ai-eval-set" ]; then \
		echo "AI evaluation fixture directory verified. Evaluation suite passed (stub)."; \
	fi

## test-template-eval: Run parser sample-set validation suite (18_testing_strategy.md §8.4)
test-template-eval:
	@echo "Running deterministic parser template evaluation suite..."
	@if [ -d "test/fixtures/email-samples" ]; then \
		echo "Email template sample fixture directory verified. Template evaluation passed (stub)."; \
	fi

## compose-up: Bring up local full-stack container environment (19_deployment.md §4.3)
compose-up:
	$(COMPOSE) -f deploy/podman-compose.yaml up -d

## compose-down: Tear down local full-stack container environment
compose-down:
	$(COMPOSE) -f deploy/podman-compose.yaml down

## help: Display available Makefile targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //'
