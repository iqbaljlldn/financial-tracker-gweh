# Sprint 0 — Foundation & Tooling Setup

**Theme:** Repository scaffold, build tooling, CI skeleton, local dev environment. Nothing domain-specific ships this sprint — this sprint exists so every later sprint has a consistent place to put code.

**Primary documents:** `21_project_structure.md`, `20_coding_convention.md`, `06_architecture.md` §3–11, `19_deployment.md` §4–5

**Entry criteria:** None — this is the first sprint.

---

## Checklist

### Repository & Project Structure
- [x] Create repo root layout per `21_project_structure.md` §3 (`cmd/`, `internal/`, `db/`, `config/`, `deploy/`, `test/`, `docs/`)
- [x] `go.mod` initialized, Go 1.25.2 pinned
- [x] Copy this entire 24-document design series into `docs/design/` (version-controlled alongside code, per `21_project_structure.md` §15)
- [x] `cmd/platform/main.go` stub — accepts `--mode=api|worker` flag, no logic yet
- [x] `internal/platform/` skeleton: `config/`, `observability/`, `httpserver/`, `taskserver/`, `inboxguard/`, `outboxrelay/` (empty packages with doc comments only)
- [x] `internal/apitypes/` skeleton: `Money`, `Confidence` value-object shapes with JSON marshaling (`21_project_structure.md` §5.1)

### Tooling
- [x] `.golangci.yaml` configured: `govet`, `staticcheck`, `errcheck` **[GATE]**
- [x] Custom module-boundary import linter configured against the module map from `06_architecture.md` §4 **[GATE]** — even though no modules exist yet, configure the rule now so it's enforced from the first module's first commit
- [x] `gofmt`/`goimports` pre-commit hook
- [x] `Makefile` with all targets from `21_project_structure.md` §6 (`build`, `run-api`, `run-worker`, `sqlc-generate`, `mocks-generate`, `migrate-up/down`, `lint`, `test-unit`, `test-integration`, `test-ai-eval`, `test-template-eval`, `compose-up/down`) — targets can be stubs that no-op where nothing exists yet, but the interface should be final
- [x] `db/sqlc.yaml` root config created (no queries yet)
- [x] mockery config set up, scoped to `domain/repository.go` interfaces only (`18_testing_strategy.md` §4)

### Local Dev Environment
- [x] `deploy/Containerfile` multi-stage build skeleton (`19_deployment.md` §4.1)
- [x] `deploy/podman-compose.yaml`: Postgres, Redis, Traefik, platform binary (api+worker) **[DEFER-OK if using cloud dev DBs]**
- [x] `deploy/traefik/dynamic.yaml` skeleton routing config
- [x] `config/config.dev.yaml` skeleton with top-level keys matching the (not-yet-built) module map
- [x] `config/secrets.env.example` template (`21_project_structure.md` §7)

### CI Skeleton
- [x] GitLab CI pipeline stub with all stages from `18_testing_strategy.md` §10 wired as no-ops that pass trivially (lint → unit → integration → contract → ai-eval → vuln-scan → build-image → deploy)
- [x] Secret-scanning CI stage active from day one (`16_security.md` §10) **[GATE]**
- [x] Dependency-vulnerability scanning CI stage active from day one (`16_security.md` §11) **[GATE]**

### Documentation
- [x] `README.md`: how to run locally, how to run tests, link to `docs/design/`
- [x] PR template embedding the Section 9 checklist from `20_coding_convention.md` (even if mostly N/A right now, establish the habit)

---

## Exit Criteria

- [x] `make build` produces a binary (even if it does nothing)
- [x] `make compose-up` brings up Postgres/Redis/Traefik locally
- [x] A trivial PR passes the full (mostly stub) CI pipeline end to end
- [x] Module-boundary linter is demonstrably active (test it with a deliberately-bad import, confirm it fails)
- [x] Team can explain the four-layer shape (`domain/application/adapter/module.go`) every module will follow, per `10_backend_blueprint.md` §3
