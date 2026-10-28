# AI Blueprint
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `03_software_requirement_specification.md`, `04_domain_model.md`, `06_architecture.md`, `10_backend_blueprint.md`
**Feeds Into:** `12_gmail_connector.md`, `13_email_parser.md`, `17_observability.md`, `24_future_roadmap.md`

---

## 1. Purpose

This document specifies the full design of the platform's **AI Engine** — the component responsible for structured extraction from unknown-sender emails (SRS-FR-AI-01), categorization (SRS-FR-AI-02), confidence scoring (SRS-FR-AI-03), and prompt versioning/attribution (SRS-FR-AI-04). It defines the prompt architecture, the structured-output contract, the confidence-computation methodology, the categorization decision tree, cost/rate-limit governance, and the provider-agnostic client abstraction that `10_backend_blueprint.md`'s `understanding/adapter/external/llmclient.go` implements against.

This is the document that operationalizes the Vision's core principle: **"AI as an Interpretation Layer, Not a Gimmick"** — AI is invoked precisely where deterministic parsing cannot reach, its output is never trusted blindly, and its behavior is fully attributable and monitorable.

## 2. Scope

### 2.1 In Scope
- Prompt architecture: structure, versioning scheme, and change-management process.
- Structured-output schema (the `CandidateFinancialEvent` JSON contract the LLM must produce).
- Confidence-score computation methodology (per-field and aggregate).
- Categorization strategy and decision hierarchy.
- Provider-agnostic client abstraction design.
- Cost governance and rate-limit-aware concurrency (resolving H3 from `05_event_storming.md` at the AI-design level, complementing `06_architecture.md`'s queue-level resolution).
- PII-minimization principles for prompt construction.
- Prompt Monitoring data contract (feeding FR-ADMIN-03).

### 2.2 Out of Scope
- Template/regex deterministic parsing (the non-AI extraction path) → `13_email_parser.md`.
- Gmail-specific email fetching/parsing mechanics → `12_gmail_connector.md`.
- Specific model selection/benchmarking results (an operational, continuously-revisited decision, not fixed in this document) → tracked operationally, referenced from `17_observability.md`'s Prompt Monitoring dashboards.

---

## 3. Architecture: AI Engine Position in the Pipeline

```
IngestedMessage (unknown sender OR template extraction failed)
        │
        ▼
┌─────────────────────────────┐
│  Prompt Construction Layer    │  ← sanitizes/truncates email content,
│                               │    selects prompt version, injects
│                               │    few-shot examples if applicable
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Provider-Agnostic LLM Client │  ← abstracts OpenAI-compatible API,
│                               │    handles retries, rate limiting
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Structured Output Validator   │  ← JSON schema validation,
│                               │    re-prompt on failure (once)
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Confidence Computation Engine │  ← per-field + aggregate confidence
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Categorization Engine         │  ← user_history → global_mapping →
│                               │    ai_inference decision hierarchy
└──────────────┬───────────────┘
               ▼
        CandidateFinancialEvent
   (flows into Confidence Gating,
    per 06_architecture.md §6, §7_sequence_diagrams.md Diagram 4)
```

---

## 4. Prompt Architecture

### 4.1 Prompt Composition Structure

Every extraction prompt is composed of four distinct, independently-versionable sections:

1. **System Instruction (stable, rarely changes):** Defines the AI's role, the strict JSON-only output contract, and the platform's confidence-scoring rubric (Section 6).
2. **Schema Definition (versioned with the output contract):** The exact JSON schema the model must conform to — field names, types, enum constraints (`event_type` taxonomy from `04_domain_model.md` Section 5.2), and required-vs-optional field rules.
3. **Few-Shot Examples (versioned independently, tunable without touching system instruction):** A curated set of example emails → correct structured outputs, covering diverse `event_type`s and edge cases (ambiguous amounts, multi-currency, non-English senders). Examples are maintained as a versioned dataset, reviewed and updated based on Prompt Monitoring data (Section 10).
4. **Email Content (per-request, sanitized):** The actual email content being extracted, subject to PII-minimization rules (Section 9) and token-budget truncation.

**Why compose from independently-versioned sections rather than one monolithic prompt string:** This allows targeted iteration — e.g., adding a new few-shot example for a newly-observed edge case without touching (and thus without needing to re-validate) the system instruction or schema definition. Each section's version is recorded, and the **prompt version** (SRS-FR-AI-04) is a composite identifier (e.g., `v7` maps to a specific tuple of `{systemInstructionVersion, schemaVersion, fewShotSetVersion}`), giving fine-grained attribution.

### 4.2 Prompt Versioning Scheme

- Prompt versions are **immutable, content-addressed or monotonically-numbered artifacts** stored in a version-controlled prompt registry (a dedicated table or config-as-code repository, decided in `10_backend_blueprint.md`'s implementation, but conceptually equivalent to a migration history — no in-place mutation of an active version).
- A new prompt version must be explicitly **activated** (a deploy-time or admin-triggered configuration change) before it is used for new extractions; the previous version remains available (never deleted) for historical attribution lookups (satisfying SRS-FR-AI-04's immutable-attribution requirement).
- **Rollback is a first-class operation:** reactivating a previous version is as simple and safe as activating a new one, since old versions are never mutated or removed.

### 4.3 Prompt Change Management Process

1. Propose a change (new few-shot example, schema field addition, system instruction refinement) against a **candidate version**.
2. Run the candidate against a held-out evaluation set (historical emails with known-correct extractions, maintained as a regression test fixture — feeds directly into `18_testing_strategy.md`'s AI-specific test suite).
3. Compare aggregate confidence distribution and accuracy metrics (Section 10) between candidate and currently-active version.
4. Activate only if the candidate meets or exceeds the active version's accuracy on the evaluation set — a regression-gated promotion process, not ad hoc prompt editing in production.

---

## 5. Structured Output Contract

The LLM is instructed to return **strictly valid JSON**, matching this conceptual schema (illustrative shape — the authoritative schema is a versioned artifact per Section 4.2, but its structure mirrors `04_domain_model.md` Section 5.1's `FinancialEvent` envelope exactly, per the symmetry design decision):

```
{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "AMZN Mktp US", "normalizedName": "Amazon", "confidence": 0.85 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.70 },
  "categoryHint": { "value": "Shopping", "confidence": 0.60 },
  "typeSpecificPayload": {},
  "extractionNotes": "Amount field was ambiguous between $45.00 and a shipping fee of $4.50; selected the larger, clearly-labeled total."
}
```

**Key design choices:**
- **Per-field confidence is mandatory**, not optional — every extractable field carries its own confidence, enabling the field-level highlighting in the review-queue UX (`09_api_specification.md` Section 6.2) and the weighted aggregate computation (Section 6).
- **`extractionNotes` is a free-text field explicitly requested from the model** — this is a deliberate design choice to capture the model's own uncertainty reasoning in natural language, which is surfaced to Prompt Monitoring (Section 10) for qualitative failure analysis, even though it's never shown to end users or used programmatically.
- **Fields the model cannot determine are omitted or null, never guessed** — the system instruction explicitly forbids fabricating a plausible-sounding value for a field the model isn't confident about; a missing field with `confidence: null` is a correct output, not a failure.

### 5.1 Schema Validation & Re-Prompt Policy

Per SRS-FR-AI-01 and `06_architecture.md` Section 6 (Diagram 4): the raw model response is validated against the JSON schema **before** any downstream processing. On validation failure:
1. **One automatic re-prompt** is issued, appending a stricter corrective instruction (e.g., "Your previous response was not valid JSON matching the required schema. Return ONLY valid JSON, no markdown code fences, no explanatory text outside the JSON structure.").
2. If the re-prompt also fails validation, the message is routed to the manual review queue (not the retry/dead-letter infrastructure — this is a content-understanding failure, not a transient infrastructure failure, per SRS-FR-AI-01's explicit error-handling distinction) and flagged in Prompt Monitoring as a `schema_invalid` outcome for that prompt version.

---

## 6. Confidence Score Computation

### 6.1 Per-Field Confidence

Requested directly from the model as part of the structured output (Section 5) — the model self-reports its certainty per field, informed by the system instruction's explicit confidence rubric (e.g., "1.0 = explicitly and unambiguously stated in the email; 0.7-0.9 = strongly implied but required minor inference; 0.4-0.6 = inferred from indirect context; below 0.4 = a low-confidence guess").

**Why trust model self-reported confidence rather than only a separate, deterministic scoring pass:** Model self-reported confidence, when explicitly rubric-anchored in the prompt (rather than left to the model's default, poorly-calibrated notion of "confidence"), correlates reasonably well with actual accuracy in practice, and is dramatically cheaper than a secondary verification pass. This design accepts that self-reported confidence is imperfect and mitigates that imperfection via **calibration monitoring** (Section 6.3), not by discarding the mechanism.

### 6.2 Aggregate Confidence Computation

The aggregate confidence used for the Confidence Gate (`04_domain_model.md` INV-1) is a **weighted combination**, not a simple average, because fields differ in importance to ledger correctness:

| Field | Weight | Rationale |
|---|---|---|
| `amount` | 0.35 | The single most consequential field for a financial ledger |
| `occurredAt` | 0.15 | Important for correct period-bucketing in analytics |
| `merchant` | 0.20 | Important for categorization and merchant ranking accuracy |
| `eventType` | 0.20 | Determines downstream processing (e.g., only `Expense` feeds Budget evaluation) |
| `paymentMethod` | 0.05 | Useful but not ledger-critical |
| `categoryHint` | 0.05 | Low weight — categorization has its own independent confidence and correction path (Section 7), so a weak category hint here doesn't need to sink the whole event's confidence |

`aggregateConfidence = Σ(fieldConfidence × fieldWeight)`, computed deterministically in the Confidence Computation Engine (application code, not the LLM) from the model's per-field outputs — this keeps the gating-critical calculation auditable, testable, and independent of any model-specific quirks in how it might self-report an "overall" score if asked directly.

**Missing/null fields** are treated as `confidence = 0` for that field in the weighted sum — a missing `amount` (weight 0.35) sharply and correctly depresses aggregate confidence, since an event without a confident amount should essentially always route to review.

### 6.3 Confidence Calibration Monitoring

Prompt Monitoring (Section 10) tracks **calibration**: of all events auto-finalized at confidence bucket `[0.70, 0.80)`, what fraction were later manually overridden by the user? If self-reported confidence is well-calibrated, this fraction should be low and roughly consistent with the bucket's implied error rate. Systematic miscalibration (e.g., events at `0.75` confidence being overridden 40% of the time) is a direct, actionable signal to revise the prompt's confidence rubric or adjust the gating threshold — this closes the loop between the "confidence is a first-class value object" design decision in `04_domain_model.md` and real production accuracy.

---

## 7. Categorization Strategy

Per SRS-FR-AI-02, categorization follows a strict decision hierarchy, evaluated in order, per `07_sequence_diagrams.md` Diagram 4's `AssignCategory` step:

1. **`user_history`:** Has this specific user previously (manually or via confirmed automation) categorized a transaction from this `merchantNormalizedName`? If yes, and with reasonable recency/consistency, apply that category with high confidence (e.g., 0.9) and `source: user_history`. This is a simple, fast, non-AI lookup against the `category_assignments` history (`08_database_design.md` Section 7.3).
2. **`global_mapping`:** Is there a high-confidence, platform-wide (anonymized, aggregated across users) mapping from this merchant to a category (e.g., "Starbucks" → "Dining" is true for nearly every user)? If yes, apply with medium-high confidence (e.g., 0.75) and `source: global_mapping`. This lookup table is built and refreshed via a periodic batch job analyzing confirmed categorizations across the user base (privacy-preserving aggregation — no individual user's transaction detail is exposed by this mechanism, per `16_security.md`).
3. **`ai_inference`:** If neither prior step yields a confident result, invoke the LLM specifically for categorization (either as part of the same extraction call for unknown senders, via the `categoryHint` field in Section 5's schema, or as a standalone categorization call for template-extracted events that still lack a category, e.g., a newly-onboarded merchant with no history). Confidence here is the model's self-reported `categoryHint.confidence`.
4. **`Uncategorized` fallback:** If even AI inference yields low confidence, the event is finalized as `Uncategorized` rather than blocking ledger finalization — per SRS-FR-AI-02's explicit error-handling rule, an uncertain category must never prevent an otherwise-confident event from being recorded.

**Feedback loop:** Every `manual` categorization submitted by a user (via `PATCH /financial-events/{id}` per `09_api_specification.md`) immediately updates that user's `user_history` for future lookups (Step 1), and is aggregated (privacy-preserved) into the `global_mapping` refresh cycle (Step 2) — the categorization system continuously improves from real usage without requiring a full model retrain, since Steps 1 and 2 are simple, fast, explainable lookups that intentionally handle the majority of volume before ever reaching the LLM.

---

## 8. Provider-Agnostic Client Abstraction

Per the Vision's explicit risk mitigation (vendor dependency, Section 17 of `01_product_vision.md`) and `03_software_requirement_specification.md` Section 7's LLM provider constraint:

- The `understanding` module's domain/application layers depend on a narrow, internally-defined interface (e.g., conceptually: `LLMExtractor.Extract(ctx, prompt) (StructuredResponse, error)`) — never on a concrete provider SDK type.
- The concrete implementation (`adapter/external/llmclient.go`, per `10_backend_blueprint.md` Section 3) targets the **OpenAI-compatible API shape** specifically because this shape is supported by multiple providers (OpenAI itself, and various OpenAI-API-compatible hosted/open-weight model providers), making a provider swap primarily a configuration change (base URL, API key, model identifier via Viper config per `06_architecture.md` Section 9) rather than a code change, as long as the new provider also exposes an OpenAI-compatible structured-output/JSON-mode capability.
- **Model identifier is itself a versioned, attributed field** (`provenance.model_identifier` per `04_domain_model.md` Section 5.4) — a provider or model swap is visible in the data lineage of every subsequently-extracted event, never silently invisible.

---

## 9. PII Minimization in Prompt Construction

Because email content (containing account numbers, personal names, addresses) is sent to a third-party LLM provider, the Prompt Construction Layer applies minimization **before** content leaves the platform's boundary:

- Full account numbers are masked to their last 4 digits before inclusion (matching what's needed for `paymentMethod.hint` anyway — no full account number is ever necessary for extraction).
- Email signature blocks, footer boilerplate (unsubscribe links, legal disclaimers, marketing content) are stripped via a lightweight pre-processing pass, both reducing PII surface and reducing token cost.
- The user's own name/address, when it appears in the email body (e.g., a shipping confirmation), is not deliberately redacted (it is often necessary context for correct merchant/amount extraction), but is never used as an output field itself — the schema (Section 5) has no field for "recipient name," so even if present in the input, the model has no structural reason to reproduce it in the output beyond incidental inclusion in `extractionNotes`, which is monitored (Section 10) but not user-facing.
- This minimization strategy is documented in detail (masking rules, stripping patterns) and cross-referenced in `16_security.md`'s data-handling section, since it is as much a security/compliance control as an AI-design one.

---

## 10. Prompt Monitoring Data Contract

Per FR-ADMIN-03 and `08_database_design.md` Section 13.2's `prompt_metrics` table, every AI extraction/categorization call emits a monitoring record capturing:

| Field | Purpose |
|---|---|
| `promptVersion` | Attribution, per SRS-FR-AI-04 |
| `modelIdentifier` | Attribution and provider/model comparison |
| `aggregateConfidence` | Feeds the confidence distribution dashboard |
| `schemaValidationOutcome` | `valid_first_try` \| `valid_after_reprompt` \| `invalid` |
| `laterOverridden` | Backfilled asynchronously once/if a user submits a correction — feeds calibration monitoring (Section 6.3) |
| `latencyMs` | Operational performance tracking |
| `tokenUsage` | Cost tracking, input/output token counts |

This data contract is the direct input to the Grafana dashboards specified in `17_observability.md`'s Prompt Monitoring panel, and to the regression-gated prompt promotion process (Section 4.3).

---

## 11. Cost & Rate-Limit Governance

Complementing `06_architecture.md` Section 6.5's queue-level resolution of Hotspot H3 (the `understanding:ai` queue's hard concurrency ceiling), this document adds the AI-design-level governance:

- **Token budget per request** is capped (input truncation applied in the Prompt Construction Layer, Section 4.1's "Email Content" section, before the call is made) — both a cost control and a mitigation against pathologically large emails degrading extraction quality.
- **Per-user and platform-wide daily AI-call budgets** are configurable (Viper), with soft-limit alerting (Administration dashboard) and a hard-limit circuit breaker that routes further calls for that period directly to the manual review queue with a `budget_exceeded` reason code, rather than silently continuing to spend — this is a deliberate business-continuity safeguard, not merely a cost-optimization nicety.
- **Categorization-only calls (Section 7, Step 3) use a smaller, cheaper prompt** than full extraction calls (Section 5) — since categorization is a much narrower task than full structured extraction, using the same heavyweight prompt for both would be a wasteful default the architecture explicitly avoids.

---

## 12. Detailed Explanation: Why Categorization Is Not Simply "Ask the LLM Every Time"

A naive design would call the LLM for categorization on every single event, since LLMs are generally capable of reasonable categorization. This blueprint explicitly rejects that default because:

1. **Cost:** At the platform's target scale (SRS Section 6.5's 500 emails/sec design point), calling an LLM for categorization on every event — even the 95%+ that are trivially re-occurring merchants (grocery stores, regular subscriptions, recurring bills) — would be substantially and unnecessarily expensive.
2. **Latency:** The `user_history`/`global_mapping` lookups (Section 7, Steps 1–2) complete in single-digit milliseconds against an indexed table, versus hundreds of milliseconds to seconds for an LLM round-trip — directly protecting the SRS's finalization latency targets (Section 6.1).
3. **Determinism and user trust:** A user who has manually categorized "Starbucks" as "Coffee" (a personal preference, distinct from a "Dining" default) expects that categorization to consistently apply going forward. An LLM asked fresh each time, even with reasonable consistency, introduces unnecessary variance into something that should be a deterministic lookup once the user has expressed a preference. This directly reflects the Vision's "Deterministic Before Probabilistic" guiding principle (`01_product_vision.md` Section 8), applied here to categorization exactly as it's applied to extraction.

---

## 13. Component Responsibilities

| Component | Responsibility |
|---|---|
| Prompt Construction Layer | PII minimization, content truncation, prompt version selection and assembly |
| Provider-Agnostic LLM Client | Provider communication, retry/timeout handling, provider-swap isolation |
| Structured Output Validator | Schema conformance enforcement, re-prompt orchestration |
| Confidence Computation Engine | Deterministic, auditable weighted aggregate confidence calculation |
| Categorization Engine | Decision-hierarchy orchestration across `user_history` → `global_mapping` → `ai_inference` → `Uncategorized` |
| Prompt Monitoring pipeline | Data capture for calibration, cost, and accuracy observability |

---

## 14. Trade-offs

| Decision | Trade-off |
|---|---|
| Model self-reported per-field confidence (rubric-anchored) vs. a separate deterministic verification pass | Cheaper and faster, at the cost of relying on imperfect model self-assessment — mitigated, not eliminated, by calibration monitoring (Section 6.3). |
| Weighted (not uniform-average) aggregate confidence | More design/tuning effort (weight selection, revisitable over time) in exchange for a gating score that better reflects true ledger-correctness risk. |
| Deterministic-first categorization hierarchy | More components to build (history lookup, global mapping batch job) than "always ask the LLM," in exchange for materially lower cost, latency, and better consistency with user preference. |
| Provider-agnostic client via OpenAI-compatible shape (not a fully custom abstraction) | Slight constraint on which providers can be adopted (must support this API shape), in exchange for near-zero abstraction-layer maintenance cost, since this shape is already a de facto industry standard. |

## 15. Advantages
- The confidence and categorization design directly and concretely implements the Vision's "AI as fallback/enrichment, not default" philosophy, rather than merely gesturing at it.
- Prompt versioning with regression-gated promotion (Section 4.3) prevents the common failure mode of prompt changes silently degrading production accuracy.
- The layered categorization hierarchy is both cost-efficient and produces a better user experience (consistent application of user preference) than a purely AI-driven approach would.

## 16. Disadvantages
- The global mapping batch job (Section 7, Step 2) requires privacy-preserving aggregation infrastructure that adds operational complexity beyond a simpler per-user-only design — justified by the cross-user consistency benefit (new users benefit from patterns established by the broader user base from day one) but a real added component.
- Weighted confidence computation (Section 6.2) introduces tunable parameters (the weight table) that require periodic, deliberate revisiting using production data — an ongoing maintenance responsibility, not a "set once" design.

## 17. Future Improvements
- Explore fine-tuning a smaller, purpose-specific model for the categorization-only task (Section 11's note on cheaper prompts) if categorization call volume/cost becomes significant enough to justify the investment.
- Extend the confidence rubric with explicit multi-language calibration once non-English sender support becomes a priority, since confidence calibration may vary by source language.
- Introduce active-learning-style prioritization for which low-confidence/high-disagreement events get surfaced first in Prompt Monitoring's qualitative review, once volume outgrows manual dashboard review.

## 18. Best Practices
- No prompt change reaches production without passing through the regression-gated evaluation process (Section 4.3) — an emergency hotfix prompt change is still required to run against the evaluation set, even if expedited, never skipped entirely.
- The per-field confidence weight table (Section 6.2) must be stored as reviewable, version-controlled configuration, not hardcoded inline in the Confidence Computation Engine, so weight tuning is a lightweight, auditable change.
- PII-minimization rules (Section 9) must be unit-tested against a fixture set of representative real-world email formats, not merely reviewed by inspection.

## 19. Production Considerations
- Prompt Monitoring's calibration signal (Section 6.3) depends on the `laterOverridden` backfill — this requires a scheduled job correlating `EventOverride` records back to their originating `extraction_results` row within a reasonable window (e.g., 30 days) post-finalization; this job's reliability is itself a production monitoring concern.
- Cost governance limits (Section 11) must be set based on real observed per-email token costs and revisited as pricing or usage patterns shift — a launch-day default that goes untouched for a year is a latent cost-overrun or over-throttling risk.
- The global mapping batch job's aggregation must be re-verified against `16_security.md`'s privacy requirements whenever its methodology changes, since it is the one place individual users' categorization behavior indirectly influences data seen (in aggregate form) by other users.

---

**Next Document:** `12_gmail_connector.md` — specifies the Gmail Connector module in full implementation-ready detail: OAuth scope handling, Watch/History API integration mechanics, sender-classification heuristics, and the concrete resolution of the dual-trigger ingestion design at the connector level.
