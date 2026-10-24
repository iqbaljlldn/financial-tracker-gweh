## Description
<!-- Provide a brief, clear summary of the changes introduced in this PR/MR. -->

## Associated Work & Requirement IDs
<!-- Link related tasks, issues, or SRS requirement IDs (e.g., FR-INGEST-01, Diagram 3). -->

---

## Mandatory Engineering Review Checklist
> **Author & Reviewer Instruction:** Every item below originates from the design specification series and must be verified prior to merge. Items not applicable to this change should be marked with `N/A` alongside a brief explanation per `20_coding_convention.md` §9.

### 9.1 Architecture & Boundaries
- [ ] No cross-module import outside the sanctioned interfaces table (`10_backend_blueprint.md` §6) — CI-enforced, but reviewer double-checks intent, not just linter-pass. (`06_architecture.md` §5.3, §5.4)
- [ ] No cross-schema database query or foreign key. (`06_architecture.md` §5.2, `08_database_design.md` §16)
- [ ] Every Command Handler persists its Outbox row in the same transaction as its state change. (`06_architecture.md` §6.1, `10_backend_blueprint.md` §4)
- [ ] Every Policy Handler is a thin translator with no embedded business logic. (`10_backend_blueprint.md` §7, §13)
- [ ] Every Query Handler calls zero mutating repository methods. (`10_backend_blueprint.md` §5)

### 9.2 Data Integrity
- [ ] Every new user-scoped repository query method includes a mandatory ownership/ `userID` parameter. (`16_security.md` §6.2, §12)
- [ ] Every new Asynq task handler with a non-naturally-idempotent side effect uses the shared `inboxguard` helper. (`06_architecture.md` §6.2, `10_backend_blueprint.md` §8.2)
- [ ] Monetary fields use `numeric(18,4)`/decimal-string representation end-to-end, never `float`/JSON number. (`08_database_design.md` §20, `09_api_specification.md` §14)
- [ ] Every migration is backward-compatible with the currently-deployed application version. (`19_deployment.md` §6, §17)
- [ ] Every new `event_type`'s `type_specific_payload` schema is documented in `04_domain_model.md` before any row using it is written. (`08_database_design.md` §20)

### 9.3 AI & Parser Changes
- [ ] No prompt version activated without passing the regression-gated evaluation suite. (`11_ai_blueprint.md` §4.3, `18_testing_strategy.md` §8.3)
- [ ] No `SenderTemplate` published without passing sample-set validation. (`13_email_parser.md` §6.3, §15, `18_testing_strategy.md` §8.4)
- [ ] New `postProcessing` DSL primitives are unit-tested against edge cases before being made available to template authors. (`13_email_parser.md` §15)

### 9.4 Security & Privacy
- [ ] No new span attribute, log field, or LLM prompt field includes Tier 1/Tier 2 PII values directly (opaque IDs only). (`16_security.md` §7, `17_observability.md` §5.3, §6)
- [ ] No secret/credential is hardcoded or committed; secret-scanning CI stage passes. (`16_security.md` §10, §11)
- [ ] Any new n8n workflow proposal has the `14_workflow_engine.md` §6 governance checklist explicitly answered in the PR/change description.

### 9.5 Observability
- [ ] Every new metric is added to `17_observability.md` §4's catalog in the same change. (`17_observability.md` §16)
- [ ] Every new failure domain or critical alert has a corresponding runbook entry added to `19_deployment.md` §8 in the same change. (`19_deployment.md` §17)
- [ ] Trace context is correctly propagated through any new Outbox-event payload the change introduces. (`17_observability.md` §5.2)

### 9.6 API & Contract
- [ ] No new endpoint without a corresponding entry in `09_api_specification.md`, cross-referenced to a requirement ID. (`09_api_specification.md` §14)
- [ ] No breaking change to an existing `/v1` endpoint contract without a version-bump discussion. (`09_api_specification.md` §7.4)

### 9.7 Documentation Currency
- [ ] Any new domain invariant has a corresponding unit test added in the same change. (`18_testing_strategy.md` §18)
- [ ] Any change to a cross-module interface is reflected in `10_backend_blueprint.md` §6's table in the same change.
- [ ] Any frontmatter-affecting change to a document described elsewhere in this series updates that document in the same change.
