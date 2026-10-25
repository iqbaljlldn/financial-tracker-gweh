# Sprint 4 — Understanding I: Deterministic Parser

**Theme:** Stand up the `understanding` module's deterministic Template/Regex Parser path — the DSL, the matcher/executor, sender classification routing, the sample-set validation harness, and the first batch of real sender templates.

**Primary documents:** `13_email_parser.md` (all)

**Entry criteria:** Sprint 3 complete — `MessageIngested` events reliably flowing.

---

## Checklist

### Understanding Module — Domain Layer
- [x] `internal/understanding/domain/sender_template.go` — `SenderTemplate` aggregate
- [x] `internal/understanding/domain/candidate_financial_event.go` — the shared `CandidateFinancialEvent` value object (shape must exactly mirror the future `FinancialEvent` envelope — coordinate with Sprint 6's schema) **[GATE]**
- [x] `internal/understanding/domain/repository.go`

### Extraction Rule DSL
- [x] Rule structure implemented: `selectorType` (`css`/`regex`/`header`/`static`), `selectorExpression`, `extractionGroup`, `postProcessing` (closed enumerated set), `required` (`13_email_parser.md` §4)
- [x] `postProcessing` primitives implemented: `trimWhitespace`, `parseDecimal`, `parseDate`, `stripCurrencySymbol`, `titleCase` — each unit-tested against edge cases **[GATE]**
- [x] DSL stored as `jsonb`, never executable code **[GATE — security boundary]**

### Template Matcher & Executor
- [x] Sender pattern matching (exact/domain/wildcard subdomain)
- [x] Exactly-one-active-version-per-sender enforcement at publish time (`13_email_parser.md` §6.2)
- [x] Execution outcome classification: full match / partial match (reduced confidence) / failure (`13_email_parser.md` §5.2)
- [x] `TemplateExtractionFailed` → `onTemplateExtractionFailed` policy stub (actual AI fallback wired in Sprint 5)

### Classification Routing
- [x] `ClassifyAndRouteMessage` command — consumes the Sprint 3 heuristic, resolves known-sender vs. unknown-sender authoritatively here (not duplicated in `ingestion`) (`12_gmail_connector.md` §7.2)
- [x] Redis-cached active-template lookup for performance (`06_architecture.md` §13)

### Authoring & Validation Workflow
- [x] Sample-set validation harness: exact-match assertion per sample email, CI-runnable **[GATE]** (`13_email_parser.md` §6.1, §18.4)
- [x] Template publish operation: atomic swap (new `active=true`, old `active=false` in one transaction), old versions never deleted (`13_email_parser.md` §6.2)
- [x] `make test-template-eval` target wired to actually run this suite

### Initial Template Library (MVP Curated Senders)
- [x] Identify and author templates for the top N known senders per `02_product_requirement_document.md` §6.1's MVP scope
- [x] Each template passes sample-set validation before being marked active **[GATE — no exceptions]**

### Parser Monitoring (Observability)
- [x] `extraction_results` table populated on every attempt
- [x] Rolling failure-rate aggregation job (trailing 1h / 24h) (`13_email_parser.md` §8.1)
- [x] Alert rule: failure rate > 5% trailing 1h (`13_email_parser.md` §8.2, `17_observability.md` §8) **[GATE — wire this now, this is the whole point of the sprint]**
- [x] Grafana "Parser Monitoring" dashboard panel stood up (not just the raw metric — someone has to actually look at it) (`17_observability.md` §7)

### Frontend (Parallel Track)
- [x] No end-user-facing screen this sprint (parsing is invisible to users by design) — instead, stand up a minimal **internal** admin view (can be a bare-bones page behind the `ops` role) listing recent `extraction_results` and template failure rates, to make Parser Monitoring usable before the full `/admin` surface exists in Sprint 10
- [x] If deferring the internal view, explicitly flag this as **[DEFER-OK]** to Sprint 10, but do not defer the underlying alert rule above

### Testing
- [x] Unit tests: DSL executor against full/partial/failure fixture cases
- [x] Unit tests: each `postProcessing` primitive
- [x] Integration test: Diagram 3's happy path through template extraction (finalization comes in Sprint 6 — stub/mock the downstream consumer for now)

---

## Exit Criteria

- [x] A real sample email from each MVP-curated sender produces a correct `CandidateFinancialEvent` via the template path
- [x] No LLM call occurs anywhere in this sprint's code path **[GATE — verifies the deterministic-first design]**
- [x] Failure-rate monitoring is live and demonstrably alerts when a template is deliberately broken in a test
- [x] Template publishing cannot succeed without passing sample-set validation (demonstrate this by attempting to publish a broken template)
