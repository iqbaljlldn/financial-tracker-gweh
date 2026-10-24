# Financial Event Platform (Personal Finance Assistant)

> A production-grade, event-driven personal finance platform engineered as a clean modular monolith with schema-isolated PostgreSQL persistence, Asynq task queues, transactional outbox/inbox guarantees, and deterministic + AI-assisted ingestion.

[![Go Report Card](https://goreportcard.com/badge/github.com/iqbaljlldn/financial-tracker-gweh)](https://goreportcard.com/report/github.com/iqbaljlldn/financial-tracker-gweh)
[![Go 1.25.2](https://img.shields.io/badge/go-1.25.2-blue.svg)](https://golang.org)
[![Architecture: Modular Monolith](https://img.shields.io/badge/architecture-modular%20monolith-emerald.svg)](docs/design/06_architecture.md)

---

## Architecture Overview

The platform implements an **8-module Clean Architecture Modular Monolith** designed for independent service extractability without distributed system penalty during early iterations:

1. **`identity`** — OAuth tokens, user accounts, authentication.
2. **`ingestion`** — Gmail connector, mailbox synchronization, raw message intake.
3. **`understanding`** — Deterministic template parsing + AI extraction fallback with confidence scoring.
4. **`financialcore`** — Immutable financial ledger, aggregate roots, confidence gating (INV-1..5), overrides.
5. **`budget`** — Budget tracking, idempotent threshold alerting.
6. **`analytics`** — CQRS read-side projections, financial analytics.
7. **`engagement`** — WhatsApp notification dispatch, rate limiting, and delivery tracking.
8. **`administration`** — Append-only tamper-resistant audit logging and operational metrics.

### Key Architectural Tenets
- **Schema Isolation:** Each module owns its dedicated PostgreSQL schema; cross-schema SQL joins and cross-module direct foreign keys are forbidden ([06_architecture.md §4](docs/design/06_architecture.md)).
- **Clean Architecture:** Every module adheres to `domain/` → `application/` → `adapter/` layering ([10_backend_blueprint.md §3](docs/design/10_backend_blueprint.md)).
- **Transactional Outbox & Inbox:** Guarantees exactly-once side effects atop at-least-once Asynq task queues ([06_architecture.md §6](docs/design/06_architecture.md)).
- **Monetary Precision:** All monetary amounts use `numeric(18,4)` decimal strings end-to-end ([08_database_design.md §20](docs/design/08_database_design.md)).

---

## Documentation

The full 24-document architectural specification series is version-controlled directly alongside the codebase in [`docs/design/`](docs/design/):

- [`01_product_vision.md`](docs/design/01_product_vision.md) — Product vision, constraints, and non-negotiables.
- [`04_domain_model.md`](docs/design/04_domain_model.md) — Ubiquitous language, aggregate roots, domain invariants (INV-1..5).
- [`06_architecture.md`](docs/design/06_architecture.md) — Modular monolith style, module map, boundary enforcement, queues.
- [`08_database_design.md`](docs/design/08_database_design.md) — PostgreSQL schemas, tables, constraints, and indexing.
- [`09_api_specification.md`](docs/design/09_api_specification.md) — REST API endpoints, DTO contracts, authentication.
- [`10_backend_blueprint.md`](docs/design/10_backend_blueprint.md) — Four-layer code shape, command/query/policy patterns.
- [`18_testing_strategy.md`](docs/design/18_testing_strategy.md) — Test pyramid, coverage gates, sequence diagram integration tests.
- [`19_deployment.md`](docs/design/19_deployment.md) — Multi-stage builds, Podman compose, Traefik edge routing, runbooks.
- [`20_coding_convention.md`](docs/design/20_coding_convention.md) — Go style, error handling, naming translations, PR review checklist.
- [`21_project_structure.md`](docs/design/21_project_structure.md) — Repository directory layout and package organization.

---

## Getting Started

### Prerequisites
- **Go:** `1.25.2+`
- **Podman** or **Docker** with Compose support
- **Make**
- **golangci-lint** (`v2.13.0+`)

### Configuration
Copy the secrets environment template to create your local `.env`:
```bash
cp config/secrets.env.example config/secrets.env
```
Non-secret configuration is managed via [`config/config.dev.yaml`](config/config.dev.yaml).

### Running Locally

1. **Spin up local infrastructure (Postgres, Redis, Traefik):**
   ```bash
   make compose-up
   ```

2. **Run database migrations:**
   ```bash
   make migrate-up
   ```

3. **Run the platform binary:**
   The application builds into a single dual-mode binary (`cmd/platform`):
   ```bash
   # Run HTTP API Server (default port 8080)
   make run-api

   # Run Background Task Worker (Asynq consumer)
   make run-worker
   ```

4. **Tear down local infrastructure:**
   ```bash
   make compose-down
   ```

---

## Development & Testing Workflow

| Command | Description |
|---|---|
| `make build` | Compile the `cmd/platform` binary into `bin/platform` |
| `make lint` | Run `gofmt`, custom module-boundary import linter, and `golangci-lint` |
| `make test-unit` | Run fast unit tests with race detection (`go test -race -short ./...`) |
| `make test-integration` | Run scenario-organized integration tests against containers |
| `make test-ai-eval` | Run AI evaluation regression suite ([18_testing_strategy.md §8](docs/design/18_testing_strategy.md)) |
| `make test-template-eval` | Run parser template validation suite |
| `make sqlc-generate` | Regenerate `adapter/postgres/gen/` code from `db/sqlc.yaml` |
| `make mocks-generate` | Regenerate repository interface mocks using `mockery` |

---

## Module Boundary Enforcement

Module isolation is strictly enforced at build and lint time:
- No module may import another module's `domain/`, `application/`, or `adapter/` packages.
- Shared infrastructure (`internal/platform/`) and value-object primitives (`internal/apitypes/`) may be imported by any module.
- Tested automatically in CI via `make lint` and `cmd/lint-boundaries`.
