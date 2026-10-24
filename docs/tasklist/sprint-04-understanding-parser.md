# Sprint 4 — Understanding I: Deterministic Parser

**Theme:** Stand up the `understanding` module's deterministic Template/Regex Parser path — the DSL, the matcher/executor, sender classification routing, the sample-set validation harness, and the first batch of real sender templates.

**Primary documents:** `13_email_parser.md` (all)

**Entry criteria:** Sprint 3 complete — `MessageIngested` events reliably flowing.

---

## Checklist

### Understanding Module — Domain Layer
- [ ] `internal/understanding/domain/sender_template.go` — `SenderTemplate` aggregate
- [ ] `internal/understanding/domain/candidate_financial_event.go` — the shared `CandidateFinancialEvent` value object (shape must exactly mirror the future `FinancialEvent` envelope — coordinate with Sprint 6's schema) **[GATE]**
- [ ] `internal/understanding/domain/repository.go`

### Extraction Rule DSL
- [ ] Rule structure implemented: `selectorType` (`css`/`regex`/`header`/`static`), `selectorExpression`, `extractionGroup`, `postProcessing` (closed enumerated set), `required` (`13_email_parser.md` §4)
- [ ] `postProcessing` primitives implemented: `trimWhitespace`, `parseDecimal`, `parseDate`, `stripCurrencySymbol`, `titleCase` — each unit-tested against edge cases **[GATE]**
- [ ] DSL stored as `jsonb`, never executable code **[GATE — security boundary]**

### Template Matcher & Executor
- [ ] Sender pattern matching (exact/domain/wildcard subdomain)
- [ ] Exactly-one-active-version-per-sender enforcement at publish time (`13_email_parser.md` §6.2)
- [ ] Execution outcome classification: full match / partial match (reduced confidence) / failure (`13_email_parser.md` §5.2)
- [ ] `TemplateExtractionFailed` → `onTemplateExtractionFailed` policy stub (actual AI fallback wired in Sprint 5)

### Classification Routing
- [ ] `ClassifyAndRouteMessage` command — consumes the Sprint 3 heuristic, resolves known-sender vs. unknown-sender authoritatively here (not duplicated in `ingestion`) (`12_gmail_connector.md` §7.2)
- [ ] Redis-cached active-template lookup for performance (`06_architecture.md` §13)

### Authoring & Validation Workflow
- [ ] Sample-set validation harness: exact-match assertion per sample email, CI-runnable **[GATE]** (`13_email_parser.md` §6.1, §18.4)
- [ ] Template publish operation: atomic swap (new `active=true`, old `active=false` in one transaction), old versions never deleted (`13_email_parser.md` §6.2)
- [ ] `make test-template-eval` target wired to actually run this suite

### Initial Template Library (MVP Curated Senders)
- [ ] Identify and author templates for the top N known senders per `02_product_requirement_document.md` §6.1's MVP scope
- [ ] Each template passes sample-set validation before being marked active **[GATE — no exceptions]**

### Parser Monitoring (Observability)
- [ ] `extraction_results` table populated on every attempt
- [ ] Rolling failure-rate aggregation job (trailing 1h / 24h) (`13_email_parser.md` §8.1)
- [ ] Alert rule: failure rate > 5% trailing 1h (`13_email_parser.md` §8.2, `17_observability.md` §8) **[GATE — wire this now, this is the whole point of the sprint]**
- [ ] Grafana "Parser Monitoring" dashboard panel stood up (not just the raw metric — someone has to actually look at it) (`17_observability.md` §7)

### Frontend (Parallel Track)
- [ ] No end-user-facing screen this sprint (parsing is invisible to users by design) — instead, stand up a minimal **internal** admin view (can be a bare-bones page behind the `ops` role) listing recent `extraction_results` and template failure rates, to make Parser Monitoring usable before the full `/admin` surface exists in Sprint 10
- [ ] If deferring the internal view, explicitly flag this as **[DEFER-OK]** to Sprint 10, but do not defer the underlying alert rule above

### Testing
- [ ] Unit tests: DSL executor against full/partial/failure fixture cases
- [ ] Unit tests: each `postProcessing` primitive
- [ ] Integration test: Diagram 3's happy path through template extraction (finalization comes in Sprint 6 — stub/mock the downstream consumer for now)

---

## Exit Criteria

- [ ] A real sample email from each MVP-curated sender produces a correct `CandidateFinancialEvent` via the template path
- [ ] No LLM call occurs anywhere in this sprint's code path **[GATE — verifies the deterministic-first design]**
- [ ] Failure-rate monitoring is live and demonstrably alerts when a template is deliberately broken in a test
- [ ] Template publishing cannot succeed without passing sample-set validation (demonstrate this by attempting to publish a broken template)
