# Coding Convention
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `08_database_design.md`, `10_backend_blueprint.md`, `16_security.md`, `17_observability.md`, `18_testing_strategy.md`, `19_deployment.md`
**Feeds Into:** `21_project_structure.md`

**Naming Convention Note:** This document is itself the authoritative source for the naming rule referenced throughout the series: Go identifiers use **camelCase** (unexported) / **PascalCase** (exported, Go's standard capitalization-based export mechanism), database identifiers use **snake_case**, and JSON wire-format fields use **camelCase** — Section 4 formalizes this with the precise translation rules at each boundary.

---

## 1. Purpose

This document specifies the platform's Go coding conventions and consolidates every "must be reviewed" or "must be enforced" note scattered across the prior nineteen documents into a single, actionable **PR review checklist**. Where earlier documents said "this is a `20_coding_convention.md` review item," this document is where that promise is kept.

This is also the document a new engineer should read first before writing their first line of code for this platform — it is the practical, day-to-day companion to the architectural documents, translating their principles into concrete habits.

## 2. Scope

### 2.1 In Scope
- Naming conventions across the Go/DB/API boundary (formalizing the convention used throughout the series).
- Formatting and style baseline (gofmt/goimports, linter configuration).
- Error-handling idioms (wrapping, sentinel errors, domain error translation).
- Comment and documentation conventions.
- The consolidated PR review checklist.
- Git commit/branching conventions.

### 2.2 Out of Scope
- Package/directory layout itself (already specified) → `21_project_structure.md`.
- CI pipeline stage definitions (already specified) → `18_testing_strategy.md`, `19_deployment.md`.

---

## 3. Architecture: Where This Document Sits

```
01-09: WHAT to build (vision, requirements, domain, API)
10-15: HOW the system is structured (backend blueprint, AI, connectors, parser, workflow, notification)
16-19: HOW it's operated safely (security, observability, testing, deployment)
20:    HOW a single engineer writes a single line of code consistent with all of the above
21:    WHERE that code physically lives
```

This document is deliberately positioned as the **last** technical-depth document before the purely organizational ones (`21_project_structure.md` onward), because every convention here only makes sense in light of the architecture it serves — it is a distillation, not a new set of independent rules.

---

## 4. Naming Conventions Across Boundaries

### 4.1 The Three-Layer Translation

| Layer | Convention | Example |
|---|---|---|
| Database (PostgreSQL) | `snake_case` | `financial_events.merchant_normalized_name` |
| Go (domain/application) | `camelCase` (unexported) / `PascalCase` (exported) | `financialEvent.merchantNormalizedName` / `FinancialEvent.MerchantNormalizedName` |
| JSON (API wire format) | `camelCase` | `"merchantNormalizedName"` |

- **DB → Go:** handled exclusively by sqlc's generated code (`10_backend_blueprint.md` Section 9) — sqlc's naming conversion from snake_case columns to PascalCase Go struct fields is the single, consistent translation point; no hand-written mapping code duplicates this.
- **Go → JSON:** handled exclusively by Go struct tags (`json:"merchantNormalizedName"`) on DTOs in `adapter/http/dto.go` (`10_backend_blueprint.md` Section 3) — DTOs are explicitly distinct types from domain aggregates (never serialize a domain type directly), so this tag placement never leaks into or constrains domain-layer field naming.
- **Go internal (domain ↔ DTO):** explicit, hand-written mapping functions (per `10_backend_blueprint.md` Section 10's three-boundary-translation principle) — never a reflection-based auto-mapper, keeping every field correspondence visible and reviewable in diffs.

### 4.2 General Go Naming Rules

- Package names: short, lowercase, no underscores (`financialcore`, not `financial_core` or `FinancialCore`) — matching the module names already used throughout `06_architecture.md`'s module map.
- Interface names: named for capability, not prefixed/suffixed mechanically (`TokenProvider`, not `ITokenProvider` or `TokenProviderInterface`) — idiomatic Go, consistent with the cross-module interfaces already named in `10_backend_blueprint.md` Section 6's table.
- Command/Query/Policy handler types: `<Verb><Noun>Command`, `<Verb><Noun>Handler`, `on<EventName>` — exactly matching the file-naming convention already established in `10_backend_blueprint.md` Section 3's directory tree, so naming and file-location conventions reinforce each other rather than being independently memorized rules.
- Domain error sentinels: `Err<Condition>` (e.g., `ErrConfidenceBelowThreshold`, `ErrCurrencyMismatch`, per `10_backend_blueprint.md` Section 12) — always declared in the `domain/` package they originate from, never centralized in a generic shared errors package, keeping ownership of what can go wrong colocated with the aggregate that enforces the rule.

---

## 5. Formatting & Linting Baseline

- `gofmt`/`goimports` run as a mandatory pre-commit hook and a CI-blocking stage (`18_testing_strategy.md` Section 10's lint stage) — no formatting debate is ever a PR discussion topic; the tool is the single source of truth.
- `golangci-lint` configured with, at minimum: standard `govet`, `staticcheck`, `errcheck` (every returned error must be explicitly handled or explicitly discarded via `_ =` with a comment explaining why — silent error-swallowing is never acceptable in a financial system), and the **custom module-boundary import linter** (`06_architecture.md` Section 5.3) as a first-class, equally-blocking rule alongside the standard linters, not a separate/optional tool.
- Cyclomatic complexity threshold on Command/Policy handler functions specifically (a tighter threshold than general code) — reinforcing `10_backend_blueprint.md` Section 13's "policies are thin translators" rule with an automated, objective check rather than relying solely on code-review judgment calls.

---

## 6. Error-Handling Idioms

### 6.1 Wrapping Convention

- Every error returned up a call stack is wrapped with `fmt.Errorf("doing X: %w", err)` context **at each layer boundary it crosses** (adapter → application → back to adapter for the HTTP/task-handler response translation), per `10_backend_blueprint.md` Section 12 — never wrapped redundantly within the same layer for the same operation.
- `errors.Is`/`errors.As` are the only sanctioned ways to inspect an error's identity up the stack — never string-matching an error message (a fragile anti-pattern explicitly disallowed in review).

### 6.2 Domain vs. Infrastructure Errors

- Domain errors (Section 4.2's `Err<Condition>` sentinels) represent **expected, meaningful business outcomes** (e.g., "confidence too low" is an expected outcome the system is designed to handle via the `pending_review` path, not a bug).
- Infrastructure errors (a failed DB connection, a Gmail API timeout) are wrapped and classified for retry-eligibility (`06_architecture.md` Section 6.4's retryable-vs-non-retryable distinction) at the adapter layer, using a shared, small set of sentinel wrapper types (`ErrTransient`, `ErrPermanent`) that Asynq task handlers check via `errors.As` to decide retry behavior — this classification must happen explicitly in code, never left to Asynq's default retry-everything behavior, since a non-retryable error (e.g., a 400-class validation failure from an external API) retrying five times wastes quota/cost for a guaranteed-to-fail-again operation.

### 6.3 Never Panic in Request/Task Handling Paths

- `panic`/`recover` is reserved strictly for truly unrecoverable programmer-error conditions (e.g., a nil interface that should be structurally impossible given correct DI wiring) and is **never** used as a control-flow mechanism for expected error conditions — every HTTP handler and Asynq task handler is wrapped by a top-level recovery middleware (logging the panic and converting it to a `500`/task-failure) as a last-resort safety net, not as the primary error-handling mechanism.

---

## 7. Comment & Documentation Conventions

- Every exported type/function has a doc comment following Go's standard `// TypeName does X` convention.
- **Domain invariant enforcement code** (e.g., the method on `FinancialEvent` that enforces INV-1) must have a doc comment **explicitly citing the invariant ID and the originating document** (e.g., `// Finalize enforces INV-1 (04_domain_model.md §5.7): an event cannot be finalized without an evaluated confidence score.`) — this is a deliberate, mandatory convention unique to this codebase's domain layer, keeping the connection between code and the design-document series (which this entire project treats as living, authoritative specifications) explicit and navigable for any future engineer, rather than requiring them to independently rediscover why a piece of validation logic exists.
- Package-level doc comments (`doc.go` per package where non-trivial) summarize the package's responsibility in one or two sentences, matching the "Component Responsibilities" table language already used consistently across every document in this series — documentation vocabulary should feel continuous between the design docs and the code itself.

---

## 8. Git Conventions

- Branch naming: `<type>/<short-description>` (`feat/budget-threshold-alerts`, `fix/gmail-watch-renewal-race`), `type` drawn from a small fixed set (`feat`, `fix`, `chore`, `docs`, `refactor`).
- Commit messages follow a Conventional-Commits-style format (`type(scope): summary`), where `scope` is the module name (`financialcore`, `ingestion`, etc.) — enabling fast `git log --grep`/changelog generation scoped to a specific module, directly useful given the module-per-schema, module-per-bounded-context structure established throughout the series.
- A migration-introducing commit must include, in its message body, an explicit note confirming backward-compatibility review (`19_deployment.md` Section 6) was performed — a lightweight, convention-based reinforcement of that document's mandatory review rule.

---

## 9. The Consolidated PR Review Checklist

This is the single most load-bearing section of this document: every item below was flagged as a review obligation somewhere earlier in the series, and is collected here so no reviewer needs to remember nineteen documents' worth of scattered rules.

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
- [ ] Any frontmatter-affecting change to a document described elsewhere in this series updates that document in the same change (this series' own internal consistency rule, extended to code: code and docs must never silently drift).

---

## 10. Detailed Explanation: Why a Consolidated Checklist, Not Just "Read the Docs"

Nineteen prior documents each state their own review obligations in context, which is correct and necessary for understanding *why* each rule exists. But a reviewer under normal PR-review time pressure cannot be expected to re-read nineteen documents on every review. This document's Section 9 exists because:

1. **Checklists reliably catch what memory and good intentions do not** — this is a well-established finding in high-reliability engineering disciplines generally, and it applies directly to a system where the individual rules (module boundaries, idempotency guards, backward-compatible migrations) are each simple to state but easy to forget to *apply* consistently across dozens of PRs over months.
2. **The checklist's items are traceable back to their origin document/section** (the parenthetical citations throughout Section 9) specifically so that if a reviewer is unsure *why* a checklist item matters, they have a direct path back to the full reasoning — the checklist is a fast-path for routine review, not a replacement for understanding, and the citations preserve that escape hatch.
3. **This mirrors exactly the doc-comment convention established in Section 7** for domain invariant code — citing the originating document and section is a consistent pattern applied at both the code level (Section 7) and the process level (this checklist), reinforcing throughout the codebase and its process that this design series is a living reference, not a one-time document dump to be forgotten after kickoff.

---

## 11. Component Responsibilities

| Component | Responsibility |
|---|---|
| `gofmt`/`goimports` + pre-commit hook | Eliminate formatting as a review topic entirely |
| `golangci-lint` (incl. custom boundary linter) | Automated enforcement of Section 9.1's architectural rules where mechanically checkable |
| PR template (embedding Section 9's checklist) | Ensure the non-mechanically-checkable review items are never silently skipped |
| Doc-comment convention (Section 7) | Keep domain-layer code self-explanatory with respect to the design-document series |

---

## 12. Trade-offs

| Decision | Trade-off |
|---|---|
| Mandatory invariant-citing doc comments (Section 7) | Slightly more verbose domain-layer code, in exchange for permanently preserved traceability between code and the design rationale, reducing future "why does this check exist" archaeology. |
| A long, explicit PR checklist (Section 9) rather than trusting reviewer memory/judgment | More PR template overhead per review, in exchange for consistent enforcement of rules that are individually easy to forget under time pressure, especially for a team that grows beyond the original design-series authors. |
| Separate, explicit transient/permanent error-wrapper types (Section 6.2) rather than relying on generic retry-everything | Slightly more error-handling boilerplate at adapter boundaries, in exchange for cost/quota-efficient, correct retry behavior consistent with `06_architecture.md` §6.4's design. |

## 13. Advantages
- Section 9's checklist operationalizes nearly every design decision made across the entire nineteen-document series into something a reviewer can actually, practically apply on every single PR — this is the document that makes the rest of the series' rigor *stick* during day-to-day development rather than decaying into an ignored historical artifact.
- The three-layer naming translation (Section 4.1) with sqlc/DTO-mapping as the only translation points keeps a notoriously easy-to-get-wrong cross-boundary concern (snake_case/camelCase mismatches) fully mechanical and low-risk.
- Citing originating documents throughout (doc comments, checklist items) keeps the entire eighteen-prior-document series genuinely "alive" in the codebase, rather than a one-time artifact disconnected from ongoing implementation.

## 14. Disadvantages
- The sheer length of Section 9's checklist risks becoming itself a victim of "checklist fatigue" if not actively maintained and periodically pruned/reorganized as the system matures — a known risk of any large checklist, requiring ongoing editorial attention, not a "write once" artifact.
- The mandatory invariant-citing doc-comment convention (Section 7) adds a small amount of friction to writing new domain code, requiring the author to know (or look up) the correct document/section reference — a minor but real authoring overhead.

## 15. Future Improvements
- As more checklist items in Section 9 become fully mechanically enforced by CI (the linter, contract tests, evaluation-set gates), prune the corresponding manual-reviewer checklist items — the goal over time is for Section 9 to shrink toward only the items that genuinely require human judgment, with everything mechanically checkable migrated fully into Section 5/`18_testing_strategy.md`'s automated gates.
- Consider generating part of Section 9's checklist as an actual GitLab PR template file (automation of the checklist's *presentation*, not its content) to further reduce the chance of a reviewer skipping it.
- Revisit the cyclomatic-complexity threshold for Policy handlers (Section 5) using real measured data once a meaningful number of policies exist in production code, to ensure the threshold is calibrated rather than an arbitrary launch-day guess.

## 16. Best Practices
- Any new convention added to this document must include, where applicable, its own originating-document citation, maintaining the traceability discipline this document itself establishes.
- When a checklist item in Section 9 is found to have been missed in a merged PR (caught later, e.g., in a post-incident review per `19_deployment.md` §18), the fix should include not just correcting the code but considering whether that item needs stronger mechanical enforcement (moving it from "manual checklist" to "automated CI gate") rather than simply trusting reviewers to catch it more carefully next time.
- This document should be revisited and explicitly re-circulated to the team whenever a new module or major feature area is added, since new code surfaces often reveal gaps in the existing checklist that weren't anticipated when it was written.

## 17. Production Considerations
- The custom module-boundary linter (Section 5) and any other CI-enforced rule referenced in Section 9 must themselves be kept in sync with the actual module map (`06_architecture.md` §4) as it evolves — a stale linter configuration silently enforcing an outdated boundary is a real risk worth periodic verification.
- Checklist adherence rate (are PRs actually satisfying Section 9's items, or is it being rubber-stamped) is itself worth occasionally auditing via a sample of merged PRs, treating process-conformance the same way `17_observability.md` treats system behavior — measured, not assumed.

---

**Next Document:** `21_project_structure.md` — specifies the concrete, full Go repository directory layout: the exact file tree realizing every module/layer boundary from `06_architecture.md` and `10_backend_blueprint.md`, build tooling configuration (`Makefile`/task runner), and the repository root's top-level organization.
