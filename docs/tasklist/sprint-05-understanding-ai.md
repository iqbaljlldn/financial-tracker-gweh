# Sprint 5 — Understanding II: AI Extractor

**Theme:** Complete the `understanding` module with the AI Extractor fallback path — prompt architecture, structured-output enforcement, confidence computation, categorization engine, and the template-to-AI fallback wiring.

**Primary documents:** `11_ai_blueprint.md` (all)

**Entry criteria:** Sprint 4 complete — deterministic parser and classification routing working.

---

## Checklist

### Provider-Agnostic LLM Client
- [ ] `adapter/external/llmclient.go` targeting OpenAI-compatible API shape (`11_ai_blueprint.md` §8)
- [ ] Narrow internal interface (`LLMExtractor`) — domain/application layers depend on this, never a concrete SDK type **[GATE]**
- [ ] Base URL/API key/model identifier fully Viper-configurable — verify a provider swap requires zero code changes

### Prompt Architecture
- [ ] Four-section composition: system instruction, schema definition, few-shot examples, email content (`11_ai_blueprint.md` §4.1)
- [ ] Prompt versioning: immutable, content-addressed/monotonic versions, never mutated in place **[GATE]**
- [ ] Version activation mechanism (config-driven, rollback = reactivating a prior version)

### PII Minimization (Do This Before Any Real Email Reaches a Prompt)
- [ ] Account number masking to last 4 digits **[GATE]**
- [ ] Signature block / footer / unsubscribe boilerplate stripping
- [ ] Unit tests against a fixture set of representative email formats **[GATE]** (`11_ai_blueprint.md` §18)

### Structured Output & Validation
- [ ] Schema matching `CandidateFinancialEvent` envelope + per-field confidence (`11_ai_blueprint.md` §5)
- [ ] Schema validation before any downstream use
- [ ] One automatic re-prompt on validation failure; second failure → manual review queue, not retry/dead-letter **[GATE]**

### Confidence Computation
- [ ] Deterministic, weighted aggregate confidence calculation in application code (not trusted from the model directly) (`11_ai_blueprint.md` §6.2)
- [ ] Weight table stored as reviewable config, not hardcoded
- [ ] Missing/null field → confidence 0 for that field in the weighted sum

### Categorization Engine
- [ ] Decision hierarchy implemented in order: `user_history` → `global_mapping` → `ai_inference` → `Uncategorized` **[GATE — must short-circuit in this order, verify with tests]** (`11_ai_blueprint.md` §7)
- [ ] `user_history` lookup against `category_assignments`
- [ ] `global_mapping` privacy-preserving aggregation batch job (can be a simple scheduled job this sprint, refine later)
- [ ] Feedback loop: manual categorization immediately updates `user_history`

### Fallback Wiring
- [ ] Complete Sprint 4's stubbed `onTemplateExtractionFailed` → `InvokeAIExtractor` (`13_email_parser.md` §7)
- [ ] Verify symmetric output: template-path and AI-path `CandidateFinancialEvent`s are structurally identical to downstream consumers

### Cost & Rate-Limit Governance
- [ ] `understanding:ai` queue with hard concurrency ceiling matched to provider rate limit **[GATE — H3 resolution]** (`06_architecture.md` §6.5)
- [ ] Token budget cap per request (input truncation)
- [ ] Per-user/platform daily AI-call budget with circuit breaker → review queue on exhaustion

### Evaluation Set & Regression Gate
- [ ] Evaluation set fixture directory seeded (`test/fixtures/ai-eval-set/`) covering each `event_type`, multiple currencies, ambiguous cases (`18_testing_strategy.md` §8.1)
- [ ] Field-level accuracy, schema-validity rate, calibration-proxy metrics computed in CI **[GATE]**
- [ ] `make test-ai-eval` wired to block prompt-version activation on regression

### Prompt Monitoring (Observability)
- [ ] `prompt_metrics` data contract populated per call (`11_ai_blueprint.md` §10)
- [ ] Metrics wired: `understanding_ai_extraction_total`, `understanding_ai_confidence_distribution`, `understanding_ai_token_usage_total`, `understanding_ai_queue_depth`, `understanding_ai_daily_budget_consumed_ratio` (`17_observability.md` §4.2)
- [ ] Alert rule: AI queue backpressure (depth/oldest-task-age threshold) — H3's queue-level signal, pair with Sprint 4's failure-rate alert (`17_observability.md` §8)
- [ ] Trace spans on every LLM provider call, tagged `external_system=llm_provider`, **no PII in span attributes** (`17_observability.md` §5.3)

### Frontend (Parallel Track)
- [ ] No new end-user screen required this sprint — AI extraction is backend-invisible same as templates. If the Sprint 4 internal admin view was deferred, build it now, extended with AI-path metrics (confidence distribution, fallback rate) since both paths' data exists by this point

### Testing
- [ ] Unit tests: confidence weighting calculation
- [ ] Unit tests: categorization hierarchy short-circuiting correctly at each tier
- [ ] Integration test: Diagram 4's unknown-sender AI extraction + low-confidence pending-review path

---

## Exit Criteria

- [ ] An email from an unknown sender produces a correct, confidence-scored `CandidateFinancialEvent` via the AI path
- [ ] A deliberately-broken template (Sprint 4) correctly falls back to AI transparently
- [ ] No prompt version can be activated without passing the evaluation-set regression gate (demonstrate with a deliberately-regressed candidate prompt)
- [ ] PII-minimization unit tests pass against the full fixture set
