# Project Structure
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `08_database_design.md`, `10_backend_blueprint.md`, `20_coding_convention.md`
**Feeds Into:** `22_risk_analysis.md`, `23_scalability.md`, `24_future_roadmap.md`

---

## 1. Purpose

This document specifies the **concrete, full Go repository directory layout**: the exact file tree realizing every module/layer boundary from `06_architecture.md` and `10_backend_blueprint.md`, build tooling configuration, and the repository root's top-level organization. Where `10_backend_blueprint.md` described the per-module internal shape in the abstract, this document shows the complete, literal tree — the single artifact a new engineer opens first to orient themselves physically within the codebase.

## 2. Scope

### 2.1 In Scope
- Full repository root directory tree.
- Per-module directory tree (expanding `10_backend_blueprint.md` Section 3 to its complete, concrete form across all eight modules).
- Shared/internal package organization (`internal/`).
- Build tooling (`Makefile`) target catalog.
- Configuration file organization (per-environment Viper config files).
- Generated-code organization (sqlc, mockery outputs).

### 2.2 Out of Scope
- File *contents* (no implementation code, per the project's standing directive).
- CI pipeline YAML itself (already specified at the process level) → `18_testing_strategy.md`, `19_deployment.md`.

---

## 3. Architecture: Repository Root Layout

```
financial-event-platform/
├── cmd/
│   └── platform/
│       └── main.go                    # Composition root; reads --mode=api|worker
│
├── internal/
│   ├── identity/                      # Module (Section 4)
│   ├── ingestion/                     # Module
│   ├── understanding/                 # Module
│   ├── financialcore/                 # Module
│   ├── budget/                        # Module
│   ├── analytics/                     # Module
│   ├── engagement/                    # Module
│   ├── administration/                # Module
│   │
│   ├── platform/                      # Cross-cutting, non-domain shared code
│   │   ├── config/                    # Viper loading/validation helpers
│   │   ├── observability/             # OTel/Prometheus/slog composition-root wiring
│   │   ├── httpserver/                # chi router setup, shared middleware
│   │   ├── taskserver/                # Asynq server/client setup, queue topology config
│   │   ├── inboxguard/                # Shared idempotency-guard helper (06_architecture.md §6.2)
│   │   └── outboxrelay/               # Shared, generic Outbox Relay poller logic
│   │       # (instantiated per-module via each module's adapter/outbox/relay.go)
│   │
│   └── apitypes/                      # Shared, minimal cross-module DTO primitives
│       # (Money, Confidence — value-object-shaped types reused across module DTOs
│       #  to avoid each module reinventing e.g. decimal-string Money marshaling)
│
├── db/
│   ├── migrations/                    # Goose migration files, one subdirectory per schema
│   │   ├── identity/
│   │   ├── ingestion/
│   │   ├── understanding/
│   │   ├── financialcore/
│   │   ├── budget/
│   │   ├── analytics/
│   │   └── engagement/
│   │       # administration's audit_log/metrics tables migrate alongside;
│   │       # see 08_database_design.md §13 for its schema
│   └── sqlc.yaml                      # Root sqlc config, per-module query/gen path entries
│
├── config/
│   ├── config.dev.yaml
│   ├── config.staging.yaml
│   └── config.production.yaml         # Structure only — secrets injected via env, per
│                                       # 06_architecture.md §9 / 16_security.md §10
│
├── deploy/
│   ├── Containerfile                  # Podman multi-stage build (19_deployment.md §4)
│   ├── podman-compose.yaml
│   └── traefik/
│       └── dynamic.yaml               # Routing/TLS config (19_deployment.md §4.3)
│
├── test/
│   ├── fixtures/
│   │   ├── email-samples/             # 13_email_parser.md §6.1 sample sets
│   │   ├── ai-eval-set/               # 11_ai_blueprint.md §4.3 / 18_testing_strategy.md §8
│   │   └── e2e-scenarios/             # 07_sequence_diagrams.md-derived E2E fixtures
│   └── integration/                   # Cross-module integration test suites
│       # (kept separate from per-module unit tests, which live alongside their
│       #  module per Go convention — see Section 4)
│
├── docs/
│   └── design/                        # This entire 24-document series lives here,
│                                       # version-controlled alongside the code it specifies
│
├── .golangci.yaml                     # Linter config, incl. custom module-boundary rule
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## 4. Per-Module Directory Tree (Full Expansion)

Expanding `10_backend_blueprint.md` Section 3 to its literal, complete form — shown once in full for `financialcore` (the core domain, warranting the most detailed illustration) with the other seven modules following the identical shape (noted in Section 4.2):

```
internal/financialcore/
├── domain/
│   ├── financial_event.go             # Aggregate root, invariants (INV-1..INV-5)
│   ├── event_override.go              # EventOverride entity
│   ├── category_assignment.go         # CategoryAssignment entity
│   ├── valueobject_money.go           # Money value object
│   ├── valueobject_confidence.go      # Confidence value object
│   ├── valueobject_provenance.go      # Provenance value object
│   ├── event.go                       # Domain event type definitions
│   ├── errors.go                      # Err<Condition> sentinels (20_coding_convention.md §4.2)
│   └── repository.go                  # Repository INTERFACES only
│
├── application/
│   ├── command/
│   │   ├── evaluate_confidence_gate.go
│   │   ├── confirm_pending_review_event.go
│   │   ├── submit_event_override.go
│   │   └── archive_financial_events.go
│   ├── query/
│   │   ├── list_financial_events.go
│   │   ├── get_financial_event_by_id.go
│   │   └── list_event_overrides.go
│   └── policy/
│       ├── on_financial_event_candidate_created.go
│       └── on_user_data_purge_requested.go
│
├── adapter/
│   ├── http/
│   │   ├── handler.go
│   │   └── dto.go
│   ├── task/
│   │   └── handler.go
│   ├── postgres/
│   │   ├── repository.go
│   │   ├── gen/                       # sqlc-generated code (gitignored, regenerated via Makefile)
│   │   └── queries/
│   │       ├── financial_events_write.sql
│   │       ├── financial_events_read.sql
│   │       ├── event_overrides.sql
│   │       └── category_assignments.sql
│   └── outbox/
│       └── relay.go
│
├── module.go                          # NewModule(deps) composition
└── *_test.go                          # Unit tests, colocated per Go convention
    # (e.g., financial_event_test.go, evaluate_confidence_gate_test.go)
```

### 4.1 Mockery-Generated Mocks Location

Per `18_testing_strategy.md` Section 4's convention (mocks generated only for `domain/repository.go` interfaces), generated mocks live in a sibling `domain/mocks/` directory (e.g., `internal/financialcore/domain/mocks/financial_event_repository.go`), gitignored and regenerated via the Makefile's `mocks` target (Section 6) — kept physically close to the interfaces they mock, never centralized in a single cross-module mocks directory, reinforcing the module-boundary discipline even in generated test scaffolding.

### 4.2 Shape Consistency Across All Eight Modules

Every other module (`identity`, `ingestion`, `understanding`, `budget`, `analytics`, `engagement`, `administration`) follows this **exact same four-directory shape** (`domain/`, `application/{command,query,policy}/`, `adapter/{http,task,postgres,outbox}/`, `module.go`), with two documented, intentional variations:

- **`analytics`** has no `application/command/` subdirectory at all (per `10_backend_blueprint.md` Section 5's CQRS-read-side design — it has only `query/` and a `policy/` that handles projection updates) and no `adapter/outbox/` (it is a terminal consumer, not an event emitter to further downstream modules in the current design).
- **`administration`** has a minimal `application/command/` (per `10_backend_blueprint.md` Section 4's note that it is primarily event-subscriber-driven) and an `adapter/postgres/` that is append-only by permission (`16_security.md` Section 8.2) — reflected in that module's repository interface exposing no `Update`/`Delete` methods at all, a structural (not just permission-level) enforcement of the audit log's tamper-resistance.

Modules with external dependencies additionally have `adapter/external/`:
- `ingestion/adapter/external/gmailclient.go` (`12_gmail_connector.md` Section 3.2)
- `understanding/adapter/external/llmclient.go` (`11_ai_blueprint.md` Section 8)
- `engagement/adapter/external/whatsappclient.go` (`15_notification_system.md`)
- `identity/adapter/external/googleoauthclient.go` (`16_security.md` Section 5.1)

---

## 5. Shared Internal Packages (`internal/platform/`, `internal/apitypes/`)

### 5.1 Why These Exist Outside Any Single Module

`internal/platform/` and `internal/apitypes/` are the two deliberate, narrow exceptions to "every module is self-contained" — but they are **not** a loophole in the module-boundary rule (`06_architecture.md` Section 5), because:

- `internal/platform/*` contains **zero domain logic** — it is purely generic infrastructure wiring (how to set up an OTel tracer, how to start a chi router, the generic shape of an Outbox Relay poller) that every module's `module.go` composes with, but that contains no knowledge of `FinancialEvent`, `Budget`, or any other domain concept. It is infrastructure plumbing, analogous to importing a third-party library — except hand-written and kept in-repo for full control, consistent with the project's general "favor explicitness over external frameworks" preference (manual DI, sqlc over an ORM) established throughout `10_backend_blueprint.md`.
- `internal/apitypes/*` contains only **universally-shared value-object *wire-format* shapes** (how `Money` and `Confidence` serialize to/from JSON) — not domain logic about what a `FinancialEvent` *is*, only how a small number of genuinely cross-cutting primitive shapes are represented at the API boundary, preventing eight independent, subtly-inconsistent reimplementations of "how do we marshal a decimal-string money value" across every module's `adapter/http/dto.go`.

### 5.2 The Boundary-Enforcement Linter's Treatment of These Packages

The custom module-boundary import linter (`06_architecture.md` Section 5.3, `20_coding_convention.md` Section 5) is explicitly configured to **permit** every module importing `internal/platform/*` and `internal/apitypes/*` (an allowed exception, unlike any module-to-module import), while continuing to forbid any module importing another module's `domain/`, `application/`, or `adapter/` packages — this exception list is itself a reviewed, deliberately short allowlist in the linter's configuration, not an escape hatch that could silently grow.

---

## 6. Build Tooling: Makefile Target Catalog

| Target | Purpose |
|---|---|
| `make build` | Compile the `cmd/platform` binary |
| `make run-api` / `make run-worker` | Run locally in the respective mode (`06_architecture.md` §11) |
| `make sqlc-generate` | Regenerate all modules' `adapter/postgres/gen/` code from `db/sqlc.yaml` |
| `make mocks-generate` | Regenerate all `domain/mocks/` packages via mockery |
| `make migrate-up` / `make migrate-down` | Apply/rollback Goose migrations against the configured environment |
| `make lint` | Run `gofmt`/`goimports` check + `golangci-lint` (incl. boundary linter) |
| `make test-unit` | Run unit tests only (fast, no containers) |
| `make test-integration` | Spin up ephemeral Postgres/Redis containers, run integration suite |
| `make test-ai-eval` | Run the AI evaluation-set regression suite (`18_testing_strategy.md` §8) |
| `make test-template-eval` | Run the sample-set template validation suite (`18_testing_strategy.md` §8.4) |
| `make compose-up` / `make compose-down` | Local full-stack Podman Compose environment |

These targets are the developer-facing entry points to exactly the CI pipeline stages specified in `18_testing_strategy.md` Section 10 — a developer running `make test-integration` locally invokes the identical test suite CI runs, never a divergent "quick local version," eliminating an entire class of "works on my machine, fails in CI" friction.

---

## 7. Configuration File Organization

Per `06_architecture.md` Section 9's layered-config design: `config/config.<env>.yaml` files hold **non-secret, structural** configuration only (queue concurrency settings, confidence thresholds, poll intervals, feature flags) — organized with top-level keys matching each module's name, directly mirroring the module map, so a configuration value's location in the file tree is always predictable from knowing which module it configures. Secrets (per `16_security.md` Section 10) are never present in these files, sourced instead from environment variables at runtime, documented via a `config/secrets.env.example` template file listing required variable names with placeholder (non-functional) values, for onboarding clarity without any risk of accidental real-secret commission.

---

## 8. Detailed Explanation: Why Tests Are Colocated Per-Module (Unit) but Centralized for Integration

Section 4's tree shows `*_test.go` files colocated within each module (standard Go convention, and consistent with `18_testing_strategy.md`'s unit-testing-against-the-domain-layer-directly approach), while Section 3 shows a single centralized `test/integration/` directory for cross-cutting integration suites. This split is deliberate:

1. **Unit tests are inherently single-module** — a `financial_event_test.go` testing `FinancialEvent.Finalize()`'s invariant enforcement has no reason to live anywhere but beside the code it tests; colocating them maximizes the standard Go-tooling benefit (running `go test ./...` naturally includes them, `go test ./internal/financialcore/...` scopes to just that module during focused development).
2. **Integration tests, by their nature, frequently span multiple modules** (per `18_testing_strategy.md` Section 6.2's table — e.g., the Diagram 3 "full pipeline" test touches `ingestion`, `understanding`, `financialcore`, `analytics`, and `budget` in one test scenario) — there is no single "home module" for such a test, and forcing it into one module's directory would misleadingly suggest an ownership that doesn't reflect the test's actual cross-cutting nature. A centralized `test/integration/` directory, organized internally by *scenario* (mirroring `07_sequence_diagrams.md`'s diagram numbering, e.g., `test/integration/diagram03_template_happy_path_test.go`) rather than by module, correctly reflects that these tests validate **emergent, cross-module system behavior**, not any single module's internal correctness.

---

## 9. Component Responsibilities

| Component | Responsibility |
|---|---|
| `cmd/platform/main.go` | Sole composition root; the only file wiring cross-module dependencies |
| `internal/<module>/` (×8) | Complete, self-contained bounded-context implementation per `06_architecture.md` |
| `internal/platform/` | Generic, domain-free infrastructure wiring shared across all modules |
| `internal/apitypes/` | Narrow, shared wire-format value-object shapes only |
| `db/migrations/<schema>/` | Per-schema Goose migration history, mirroring module-schema ownership |
| `test/integration/` | Cross-module, scenario-organized integration suites |
| `Makefile` | The single source of truth for every local and CI-invoked build/test/generate operation |

---

## 10. Trade-offs

| Decision | Trade-off |
|---|---|
| Two narrow shared packages (`internal/platform`, `internal/apitypes`) as the only exceptions to full module self-containment | A small, explicitly-allowlisted exception to an otherwise-strict boundary rule, in exchange for avoiding eight redundant reimplementations of generic infrastructure wiring and money/confidence wire-format marshaling. |
| Scenario-organized centralized integration tests (not module-colocated) | Slightly less discoverable from within a single module's directory (an engineer working purely in `financialcore/` must know to look in `test/integration/` for cross-module coverage), in exchange for test organization that honestly reflects which tests validate single-module vs. emergent cross-module behavior. |
| `db/sqlc.yaml` as a single root config with per-module path entries (not eight independent `sqlc.yaml` files) | Slightly less per-module configuration independence, in exchange for a single, easily-auditable place to verify every module's generated-code path stays correctly scoped to its own schema/query directory. |

## 11. Advantages
- The full, literal directory tree removes any ambiguity a new engineer might have after reading `10_backend_blueprint.md`'s more abstract description — this document is the direct, unambiguous "where do I put this file" reference.
- The identical four-directory shape across all eight modules (Section 4.2) means familiarity with any one module transfers almost completely to any other, directly reducing onboarding time for engineers moving across module boundaries.
- Makefile targets mapping 1:1 onto CI stages (Section 6) eliminates local/CI environment drift as a recurring source of confusing, hard-to-reproduce CI failures.

## 12. Disadvantages
- The explicit, narrow shared-package allowlist (Section 5.2) requires ongoing vigilance to prevent scope creep — a team under deadline pressure might be tempted to add "just one more" shared helper that actually encodes domain logic, which the linter's allowlist mechanism alone cannot catch (only a human reviewer, informed by this document's Section 5.1 rationale, can judge that distinction).
- `administration`'s structurally-reduced shape (no Update/Delete in its repository interface, Section 4.2) means it is a meaningful exception to the "all modules identical" claim, requiring this document to be explicit about it rather than letting a new engineer discover the asymmetry by surprise.

## 13. Future Improvements
- If `understanding` is ever extracted into a standalone service (per `06_architecture.md` Section 18's noted first-extraction-candidate), this document's tree should be revised to show the extracted module's directory moving to its own repository (or a clearly-delineated subdirectory with its own `go.mod` in a monorepo approach) — this document should be the first one updated when that extraction work begins, as the concrete map of what physically moves.
- Consider adding a `scripts/` directory for one-off operational scripts (e.g., a manual GDPR-cascade verification script referenced in `18_testing_strategy.md` §19) if such scripts accumulate beyond what belongs naturally in the Makefile's target catalog.

## 14. Best Practices
- No new top-level directory should be added to the repository root (Section 3) without updating this document in the same change — this document's Section 3 tree must remain the living, accurate map of the actual repository, not a point-in-time snapshot that drifts.
- Any proposed addition to `internal/platform/` or `internal/apitypes/` must be justified against Section 5.1's "zero domain logic" / "universally-shared primitive only" test before being added — a reviewer should reject any addition that fails this test, redirecting it into the specific module(s) that actually need it instead.
- New modules (should the taxonomy ever require one — unlikely given the current eight-bounded-context map's stability, but not impossible) must follow Section 4's exact four-directory shape from their first commit, never an ad hoc structure "to be cleaned up later."

## 15. Production Considerations
- The `make sqlc-generate` and `make mocks-generate` outputs are gitignored (Section 4, Section 4.1) — the CI pipeline must regenerate them fresh on every run (never rely on a committed, potentially-stale generated-code snapshot), and this regeneration step's correctness/reproducibility should itself be a monitored CI reliability concern.
- `docs/design/` containing this entire document series version-controlled alongside the code (Section 3) means documentation and code changes can and should be reviewed together in the same PR whenever a code change has a corresponding "must update this document" obligation (per `20_coding_convention.md` Section 9.7's checklist item) — this physical colocation is what makes that checklist item practically enforceable, rather than aspirational.

---

**Next Document:** `22_risk_analysis.md` — consolidates every risk surfaced across the series (vendor dependency risk from `01_product_vision.md`, the eight event-storming hotspots, AI calibration/cost risk, WhatsApp quality-rating risk, GDPR compliance risk) into a single risk register with likelihood/impact assessment and mitigation ownership.
