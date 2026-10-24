# Testing Strategy
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `07_sequence_diagrams.md`, `08_database_design.md`, `10_backend_blueprint.md`, `11_ai_blueprint.md`, `16_security.md`
**Feeds Into:** `19_deployment.md`, `20_coding_convention.md`, `22_risk_analysis.md`

---

## 1. Purpose

This document specifies the platform's full testing strategy: the test pyramid structure, the AI-specific evaluation-set testing approach foreshadowed in `11_ai_blueprint.md`, the mandatory GDPR-cascade and idempotency test scenarios flagged repeatedly throughout the series (`05_event_storming.md` H8, `07_sequence_diagrams.md` Diagrams 5 and 8, `16_security.md` Section 9.2), and the CI pipeline's test-gating structure using `testing`, `testify`, and `mockery` per the mandated stack.

Every failure branch shown in `07_sequence_diagrams.md`, every invariant defined in `04_domain_model.md`, and every hotspot resolved in `06_architecture.md` is treated here as a **mandatory test obligation**, not an optional nice-to-have — this document is where "the design is correct" becomes "the design is verified."

## 2. Scope

### 2.1 In Scope
- Test pyramid definition (unit, integration, contract, end-to-end) with per-layer scope and tooling.
- Domain-layer invariant testing strategy.
- The AI evaluation-set regression testing approach (extending `11_ai_blueprint.md` Section 4.3).
- The deterministic-parser sample-set validation testing approach (extending `13_email_parser.md` Section 6.1).
- Idempotency and Outbox/Inbox correctness testing.
- GDPR cascade completeness testing.
- API contract testing against `09_api_specification.md`.
- CI pipeline test-gating structure and coverage requirements.
- Test data management and fixture strategy.

### 2.2 Out of Scope
- Load/performance testing methodology in full detail (referenced here, owned by `23_scalability.md`).
- Manual QA/exploratory testing processes (an operational practice, not a design artifact).

---

## 3. Architecture: The Test Pyramid

```
                    ▲
                   ╱ ╲
                  ╱E2E╲              Few, slow, highest confidence
                 ╱─────╲             (full sequence-diagram flows,
                ╱Contract╲            §7)
               ╱───────────╲
              ╱ Integration  ╲       Moderate count, per-module,
             ╱─────────────────╲     real Postgres/Redis (§6)
            ╱                    ╲
           ╱        Unit           ╲  Many, fast, per Command/Query/
          ╱───────────────────────  ╲ Policy handler and domain type (§5)
         ▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔
```

This shape directly mirrors `10_backend_blueprint.md` Section 13's command/query/policy package separation: that architectural decision was justified partly *because* it produces exactly this testable shape — Command Handlers unit-testable without a queue, Policy handlers unit-testable as thin translators, and the full reactive chain integration/E2E-testable when needed.

---

## 4. Testing Tooling (Per Mandated Stack)

| Tool | Role |
|---|---|
| `testing` (Go standard library) | Base test runner and assertions |
| `testify` | Assertion ergonomics (`assert`/`require`) and test suite structuring |
| `mockery` | Generated mocks for repository/gateway interfaces (`04_domain_model.md`'s `domain/repository.go` interfaces, per `10_backend_blueprint.md` Section 10) |

**Convention:** `mockery` generates mocks **only for interfaces defined in `domain/`** (repository interfaces, cross-module published interfaces per `10_backend_blueprint.md` Section 6) — never for concrete structs, keeping the mocking boundary aligned exactly with the Clean Architecture dependency-inversion points already established.

---

## 5. Unit Testing

### 5.1 Domain Layer

Every invariant enumerated in `04_domain_model.md` Section 5.7 (INV-1 through INV-5) must have a corresponding, explicitly-named unit test against the domain aggregate's own methods — **not** against a repository or database, since these are pure, in-memory business-rule tests:

| Invariant | Required Test Case(s) |
|---|---|
| INV-1 (confidence gate) | Finalizing without an evaluated confidence returns a domain error; finalizing at/above threshold succeeds; finalizing below threshold correctly routes to `pending_review` |
| INV-2 (currency immutability) | An override attempting to change `amount.currency` is rejected with a specific domain error, distinct from a same-currency amount correction, which succeeds |
| INV-3 (dedup uniqueness) | Tested primarily at the integration layer (Section 6, since it's a database constraint), but the domain-level "does this candidate already exist" check logic, if any exists above the DB constraint, is unit-tested here |
| INV-4 (archived exclusion) | An archived event is correctly excluded from a domain-level "is this event eligible for analytics" predicate, if such logic exists in the domain layer (vs. purely in the Analytics query layer) |
| INV-5 (event_type mutability via override only) | A direct mutation attempt outside the `EventOverride` path is structurally impossible (compile-time, via unexported fields) — verified by the domain type's API surface itself, reinforced by a test confirming `event_type` change is only reachable via the override method |

### 5.2 Application Layer (Command/Query/Policy Handlers)

- **Command handlers:** Unit-tested with `mockery`-generated repository mocks — verifying that, given a valid Command, the handler calls the expected domain methods and persists via the expected repository calls, **including the Outbox row insertion within the same transaction call** (verified via mock expectation on the unit-of-work interface, per `10_backend_blueprint.md` Section 4).
- **Query handlers:** Unit-tested with mocked read-repositories, verifying correct query parameter translation and result mapping — and explicitly verifying **no mutating repository method is ever called** (a `mockery` expectation asserting zero calls to any write method, operationalizing `10_backend_blueprint.md` Section 5's "query handlers never mutate" rule as an enforced test, not just a convention).
- **Policy handlers:** Unit-tested by constructing a hand-built domain event payload and asserting the correct Command is issued to the (mocked) Command Handler — per `10_backend_blueprint.md` Section 7's "policies are thin translators" design, this test should be trivially simple; a Policy handler test that's *not* trivially simple is itself a code-smell signal that business logic has leaked into the policy layer, worth flagging in review.

### 5.3 Coverage Requirement

- Domain layer: **≥90%** line coverage (the highest bar, since this is the highest-stakes, purest-logic layer).
- Application layer: **≥80%** line coverage.
- Adapter layer: Lower bar (**≥60%**), since much of this layer is thin translation/wiring better validated by integration tests (Section 6) than exhaustive unit mocking.

These are CI-enforced gates (Section 10), not aspirational targets.

---

## 6. Integration Testing

### 6.1 Scope

Integration tests run against **real PostgreSQL and Redis instances** (containerized, ephemeral, per-test-run — via Podman in CI, matching the production container runtime for maximum fidelity) — never against mocks, since the entire point of this layer is verifying real database constraint behavior (unique constraints enforcing INV-3, `FOR UPDATE SKIP LOCKED` behavior in the Outbox Relay, actual Asynq task enqueue/dequeue via real Redis).

### 6.2 Mandatory Integration Test Scenarios

Directly derived from `07_sequence_diagrams.md`'s eight diagrams — **every diagram in that document is a required integration test scenario**, including its documented failure branch:

| Sequence Diagram | Required Integration Test(s) |
|---|---|
| Diagram 1 (Onboarding) | Successful linking; duplicate-email-linking conflict (H1) returns `409`, no duplicate row created |
| Diagram 2 (Real-time ingestion) | Push-triggered incremental sync processes correctly; independently, the scheduled poll-fallback also processes correctly when run with no push having occurred (H2 verification) |
| Diagram 3 (Template happy path) | Full pipeline from `IngestedMessage` to Analytics projection update, asserting final state at every table touched |
| Diagram 4 (AI fallback + review) | Low-confidence path correctly creates `pending_review` status; `ConfirmPendingReviewEvent` correctly finalizes; category-at-candidate-time (H5) is asserted present even on the pending-review record |
| **Diagram 5 (Duplicate delivery / Inbox pattern)** | **Mandatory, highest-priority test:** simulate a redelivered Asynq task for an already-finalized event; assert exactly one `financial_events` row exists, and the redelivery completes as a successful no-op (not an error) |
| Diagram 6 (Budget alert) | Threshold crossing fires exactly one alert; a second event crossing the same already-fired threshold does **not** re-fire (idempotent alerting via `budget_fired_alerts` unique constraint) |
| Diagram 7 (Notification retry) | Transient WhatsApp API failure triggers retry; eventual success correctly updates `delivery_status`; a simulated permanent failure (e.g., blocked number) is classified correctly per `15_notification_system.md` Section 8 |
| **Diagram 8 (Purge cascade)** | **Mandatory, compliance-critical test:** full cascade from `RequestDataPurge` through `financial_events.status = archived`, Analytics exclusion, and `audit_log` insertion; explicitly asserts the audit log entry **survives** a subsequent simulated permanent-erasure sweep while the underlying `financial_events` rows do not (verifying `16_security.md` Section 8.3's exemption) |

### 6.3 Cross-Module Boundary Tests

A dedicated integration test suite verifies the module-boundary enforcement itself: attempting to construct a scenario that would require a forbidden cross-schema query fails at the CI-linting stage (Section 10), but this suite additionally verifies the **sanctioned** cross-module interfaces (`10_backend_blueprint.md` Section 6's table) behave correctly end-to-end (e.g., `TokenProvider`'s `identity`→`ingestion` flow genuinely returns a valid, refreshed token across a real database-backed token record).

---

## 7. Contract Testing

### 7.1 API Contract Tests

Per `09_api_specification.md` Section 15's directive: a dedicated CI stage runs generated contract tests validating that every implemented HTTP handler's actual request/response shape matches the OpenAPI artifact derived from `09_api_specification.md` — using a schema-validation-based contract test (request/response bodies validated against the OpenAPI schema for each endpoint) rather than hand-maintained duplicate assertions, so contract drift is caught mechanically.

### 7.2 External Provider Contract Tests

For each external dependency (Gmail API, WhatsApp Business API, LLM provider), a small set of **recorded-response-based contract tests** (using recorded/fixture HTTP responses, not live calls in CI) verify the platform's client code correctly handles: successful responses, documented error responses (rate limits, auth failures), and malformed/unexpected responses — protecting against silent breakage if a provider's response shape changes, without making CI dependent on live third-party availability.

---

## 8. AI-Specific Testing: Evaluation-Set Regression Suite

Directly implementing `11_ai_blueprint.md` Section 4.3's regression-gated prompt promotion process:

### 8.1 Evaluation Set Composition

- A curated, version-controlled set of representative email fixtures (anonymized/synthetic where real data can't be used, per `16_security.md` Section 7's PII handling extended to test fixtures) covering: each `event_type` in the taxonomy, multiple currencies, ambiguous-amount edge cases, non-English sender examples, and known historically-problematic cases (each production `schema_invalid` or significantly-miscalibrated extraction, once diagnosed via Prompt Monitoring per `17_observability.md`, is added to this set as a permanent regression fixture — the evaluation set **grows from real production learnings**, not just initial authoring).

### 8.2 Evaluation Metrics

For a candidate prompt version, computed against the evaluation set:
- **Field-level accuracy:** percentage of fixtures where each field's extracted value exactly matches (or is within tolerance, for dates/amounts) the fixture's known-correct value.
- **Schema-validity rate:** percentage of fixtures producing valid structured output on the first attempt (without needing the re-prompt from `11_ai_blueprint.md` Section 5.1).
- **Confidence calibration proxy:** since the evaluation set has known-correct answers, confidence-vs-correctness correlation can be directly measured here (a stronger signal than the production-only calibration monitoring in `11_ai_blueprint.md` Section 6.3, which relies on the noisier "was later overridden" proxy) — evaluation-set calibration testing is the pre-production complement to production calibration monitoring.

### 8.3 Promotion Gate

A candidate prompt version may only be activated (per `11_ai_blueprint.md` Section 4.2) if it meets or exceeds the currently-active version's scores on all three metrics above, run as an automated CI stage — this is the mechanical enforcement of the "regression-gated promotion" process that document described at the process level.

### 8.4 Deterministic Parser Sample-Set Testing

Analogously, per `13_email_parser.md` Section 6.1: every `SenderTemplate` version's sample-set validation (exact-match assertion against known-correct extraction for every sample email) is itself a CI-runnable test suite — a template cannot be published (Section 6.3 of that document) without this suite passing, mechanically enforced the same way as Section 8.3 above for prompts.

---

## 9. End-to-End (E2E) Testing

- A small number of full-stack E2E tests (real containerized Postgres/Redis/mock-external-providers, exercising the complete HTTP API surface) cover the platform's highest-value user journeys: full onboarding-to-first-notification flow, and the GDPR purge-cascade flow (Diagram 8, run once more here at full E2E fidelity as the single highest-stakes scenario in the platform, deliberately tested at both the integration layer, Section 6.2, and again at full E2E, given its compliance criticality).
- E2E tests are deliberately few (per the pyramid shape, Section 3) and run less frequently in CI (e.g., on merge to main / nightly) than the unit/integration layers (every commit), balancing thoroughness against CI feedback-loop speed.

---

## 10. CI Pipeline Test-Gating Structure

| Stage | Trigger | Blocking? |
|---|---|---|
| Lint (incl. module-boundary import linter, `06_architecture.md` §5.3) | Every push | Yes |
| Unit tests + coverage gate (§5.3) | Every push | Yes |
| Integration tests (§6) | Every push (containerized Postgres/Redis spun up in-pipeline) | Yes |
| Contract tests (§7) | Every push | Yes |
| AI evaluation-set regression (§8) | Every push touching prompt/template configuration; scheduled nightly otherwise | Yes (when triggered) |
| Dependency/secret vulnerability scan (`16_security.md` §11) | Every push | Yes |
| E2E tests (§9) | Merge to main + nightly schedule | Yes on main; informational on feature branches |

**No merge to a protected branch is permitted with any blocking stage failing** — this is the concrete enforcement of the "never simplify the design" and "production-grade" directives at the process level: a document series this rigorous is only meaningful if the CI pipeline actually holds the codebase to it.

---

## 11. Test Data & Fixture Management

- Fixture data (email samples, evaluation sets, sequence-diagram scenario setups) lives in a version-controlled `testdata/` directory structure mirroring the module layout from `10_backend_blueprint.md` Section 3, with AI/parser evaluation fixtures specifically isolated (per Section 8's PII-handling note) from general integration-test fixtures.
- **Synthetic data generation utilities** are maintained for load/scale testing scenarios (`23_scalability.md`) and for populating realistic-volume staging environments without using real user data — a deliberate investment given the platform's sensitive data classification (`16_security.md` Section 7).
- Test database migrations use the **same Goose migration files** as production (never a parallel/simplified test schema) — ensuring integration tests are validating against the real, current schema at all times, with zero risk of test/production schema drift.

---

## 12. Detailed Explanation: Why the Purge Cascade Is Tested at Both Integration and E2E Layers

Most flows in this platform are tested once, at the layer where that's most efficient (per the standard pyramid economics — push testing as far down/fast as correctness allows). The GDPR purge cascade (`07_sequence_diagrams.md` Diagram 8) is a deliberate, documented exception, tested redundantly at both Section 6.2 and Section 9:

1. **Legal/compliance consequence asymmetry:** Nearly every other bug in this platform, if it slips through, produces a product-quality or revenue problem — bad, but recoverable and bounded. A partial-cascade purge bug produces a **direct regulatory compliance failure** with potential legal exposure (`16_security.md` Section 9.2) — the cost of an escaped bug here is categorically different from elsewhere in the system, justifying redundant test investment that would be excessive for a typical feature.
2. **The integration test (Section 6.2) verifies the cascade's internal correctness** (does each module's state change exactly as designed, in the right order, with the right exemption for `audit_log`) at high speed and fidelity to real database behavior. **The E2E test (Section 9) additionally verifies the cascade is correctly *triggerable and observable* through the actual public API surface** (`09_api_specification.md`'s `DELETE /me/gmail-accounts/{id}?purge=true`) exactly as a real user/client would invoke it — catching any gap between "the internal logic is correct" and "the internal logic is correctly wired to the thing a user can actually do," which is precisely the kind of gap that has caused real-world GDPR-compliance incidents at other companies (a user-facing "delete my data" button that doesn't actually trigger the correct internal flow due to a wiring bug, despite the underlying logic being sound).

---

## 13. Component Responsibilities

| Component | Responsibility |
|---|---|
| `mockery`-generated mocks | Enable fast, isolated unit testing against `domain/repository.go` interfaces |
| Containerized integration test harness | Real-fidelity verification of database constraints and Outbox/Inbox behavior |
| OpenAPI-schema-based contract tests | Mechanical drift detection between `09_api_specification.md` and actual handler behavior |
| AI/template evaluation-set CI stages | Regression-gated promotion enforcement for both extraction paths |
| CI pipeline gating (Section 10) | The enforcement mechanism making every other section of this document actually binding |

---

## 14. Trade-offs

| Decision | Trade-off |
|---|---|
| Real Postgres/Redis for integration tests (not mocks/in-memory substitutes) | Slower CI feedback loop than a fully-mocked approach, in exchange for genuine confidence in database-constraint-dependent correctness (INV-3, idempotent alerting) that mocks cannot verify. |
| Redundant purge-cascade testing (integration + E2E) | Extra test-maintenance burden for one specific flow, in exchange for defense-in-depth against the platform's single highest-consequence bug category. |
| Growing the AI evaluation set from production-diagnosed failures | Requires an operational discipline (someone must actually add the fixture after diagnosing a production issue) rather than a "build once" static set, in exchange for an evaluation set that becomes more representative of real failure modes over time rather than staying frozen at launch-day assumptions. |

## 15. Advantages
- Every diagram in `07_sequence_diagrams.md` and every invariant in `04_domain_model.md` has an explicit, named test obligation here — closing the loop from design documentation to verified implementation with full traceability.
- The regression-gated promotion CI stages (Section 8) make `11_ai_blueprint.md` Section 4.3's and `13_email_parser.md` Section 6.3's process descriptions mechanically enforced, not merely policy statements trusted to human discipline.
- The pyramid shape, directly enabled by `10_backend_blueprint.md`'s command/query/policy separation, keeps the fast-feedback unit layer large and the slow, high-fidelity E2E layer appropriately small and targeted.

## 16. Disadvantages
- The full test suite (particularly integration + AI evaluation stages) represents a nontrivial CI runtime cost, requiring ongoing pipeline-performance attention (parallelization, caching) to keep feedback loops fast enough to not degrade developer velocity — an operational cost that must be actively managed, not merely accepted once.
- Maintaining synthetic/anonymized fixture data (Section 11) at sufficient realism to catch genuine edge cases requires ongoing curation effort, distinct from and additional to normal test-writing effort.

## 17. Future Improvements
- Introduce mutation testing (verifying that unit tests actually fail when domain logic is deliberately broken) for the highest-stakes domain invariant tests (Section 5.1), providing a stronger confidence signal than line-coverage percentage alone.
- Expand contract testing (Section 7.2) to include periodic **live** (not just recorded-fixture) smoke tests against each external provider's sandbox/staging environment, catching real provider-side changes faster than waiting for a production incident to reveal a fixture is stale.
- Build a dedicated "chaos"-style test suite specifically targeting the failure-domain table from `06_architecture.md` Section 12 (simulated Redis outage, simulated KMS unavailability) once the platform's operational maturity and the value of this investment justify it beyond MVP.

## 18. Best Practices
- No new domain invariant may be added to `04_domain_model.md` without a corresponding unit test added to this document's Section 5.1 table in the same change — the two documents must never drift.
- No new `SenderTemplate` or prompt version may be activated without passing its respective evaluation-set/sample-set CI gate (Section 8) — no manual override or "just this once" exception path should exist in the pipeline configuration.
- Every production incident post-mortem (`19_deployment.md`) must include an explicit "was this covered by an existing test, and if not, has a regression test been added" checklist item, ensuring the test suite continuously absorbs real-world learnings rather than remaining static.

## 19. Production Considerations
- CI pipeline runtime and reliability are themselves a production-adjacent concern — a flaky or slow test suite erodes the entire discipline this document establishes by encouraging engineers to bypass or ignore it; pipeline health should be monitored with the same rigor as the application itself (`17_observability.md`'s philosophy extended to CI).
- The evaluation-set and sample-set fixture stores (Section 11) must be included in the platform's data-handling and backup considerations despite being "just test data," given they contain real (anonymized) financial-notification content subject to the same PII-handling discipline as production data.
- Before launch, a full dry-run of the GDPR purge cascade (Section 12) against a staging environment with production-representative data volume is a mandatory, sign-off-gated activity — not merely passing the automated test suite, but a deliberate, observed operational rehearsal, given the compliance stakes.

---

**Next Document:** `19_deployment.md` — specifies the full deployment architecture: Podman container build/orchestration, Traefik routing/TLS configuration, GitLab CI/CD pipeline stages (extending Section 10's test-gating into the full build-deploy pipeline), environment topology (dev/staging/production), and operational runbooks for the failure domains and alerts catalogued in `06_architecture.md` and `17_observability.md`.
