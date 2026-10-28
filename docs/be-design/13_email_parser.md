# Email Parser
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `06_architecture.md`, `08_database_design.md`, `10_backend_blueprint.md`, `11_ai_blueprint.md`, `12_gmail_connector.md`
**Feeds Into:** `17_observability.md`, `18_testing_strategy.md`, `24_future_roadmap.md`

---

## 1. Purpose

This document specifies the **deterministic Template/Regex Parser** — the primary, cost-efficient, high-reliability extraction path invoked for known senders (SRS-FR-PARSE-01), and the concrete mechanism by which it falls back to the AI Extractor (`11_ai_blueprint.md`) on failure. It defines the `SenderTemplate` authoring and versioning workflow, the extraction rule DSL (domain-specific language) used to express deterministic extraction logic, and the Parser Monitoring failure-rate-spike detection design that operationalizes Hotspot H3's early-warning requirement.

This is the document that makes concrete the Vision's "Deterministic Before Probabilistic" principle (`01_product_vision.md` Section 8) — it defines exactly how a known sender's email is turned into a `CandidateFinancialEvent` **without any LLM call**, at the reliability and cost profile the product's unit economics depend on.

## 2. Scope

### 2.1 In Scope
- The extraction rule DSL: structure, supported selector/pattern types, field-mapping conventions.
- `SenderTemplate` authoring, review, and versioning workflow.
- Template matching mechanics (how a message is matched to a template).
- Template execution and failure classification (required-field-missing vs. partial-match vs. full-match).
- The template-to-AI-fallback trigger mechanism.
- Parser Monitoring's failure-rate-spike detection design (H3's early-warning half).

### 2.2 Out of Scope
- AI extraction mechanics → `11_ai_blueprint.md`.
- Gmail-specific sender identification/routing → `12_gmail_connector.md`.
- Actual template rule content for specific banks/e-wallets (an ongoing content/ops deliverable, not a fixed engineering artifact) — a small illustrative example is included here for clarity only.

---

## 3. Architecture: Parser Position in the Pipeline

```
IngestedMessage (routed to template_parser, per 12_gmail_connector.md §7)
        │
        ▼
┌─────────────────────────────┐
│  Template Matcher              │  ← selects the active SenderTemplate
│                               │    version for this sender pattern
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Extraction Rule Executor       │  ← applies the DSL rules against
│                               │    the email's DOM/text structure
└──────────────┬───────────────┘
               ▼
        ┌──────┴──────┐
        ▼             ▼
  Full/Partial    Required Field
     Match           Missing
        │             │
        ▼             ▼
CandidateFinancialEvent   Fallback to
(method=template)          AI Extractor
                          (11_ai_blueprint.md)
```

---

## 4. Extraction Rule DSL

### 4.1 Design Goals

The DSL is designed to be:
- **Declarative, not procedural** — a template is a data structure (stored as `jsonb` per `08_database_design.md` Section 9.1), not executable code, so that adding/editing a template never requires a code deployment.
- **Sender-format-adaptive** — supports both HTML-structured emails (CSS-selector-based extraction) and plain-text emails (regex-based extraction), since real-world bank/e-wallet emails vary significantly in format.
- **Composable per field** — each target field (`amount`, `merchant`, `occurredAt`, `paymentMethod`, `eventTypeHint`) has its own independent extraction rule, so a partial template (e.g., missing a `paymentMethod` rule) is a valid, if incomplete, template — the executor handles partial coverage explicitly (Section 5.2), rather than requiring all-or-nothing rule authoring.

### 4.2 Rule Structure (Conceptual Shape)

Each field-level rule specifies:

| Rule Component | Purpose |
|---|---|
| `selectorType` | `css` (for HTML body) \| `regex` (for plain text or HTML-stripped-to-text) \| `header` (for extracting from email headers, e.g., sender display name) |
| `selectorExpression` | The actual CSS selector or regex pattern |
| `extractionGroup` | For regex: which capture group holds the value; for CSS: whether to use element text content or a specific attribute |
| `postProcessing` | An enumerated, safe set of transformations: `trimWhitespace`, `parseDecimal` (locale-aware), `parseDate` (with an explicit format string), `stripCurrencySymbol`, `titleCase` — **deliberately a closed, enumerated set, not arbitrary code**, keeping templates data, not scripts (a security and maintainability boundary explained in Section 10) |
| `required` | Boolean — whether a failure to extract this field constitutes a template-execution failure (triggering AI fallback) or an acceptable partial result |

### 4.3 Illustrative Example (Conceptual, Not a Literal Production Template)

```
{
  "senderPattern": "*.notifications@examplebank.com",
  "version": 3,
  "fields": {
    "amount": {
      "selectorType": "css",
      "selectorExpression": "td.transaction-amount",
      "postProcessing": ["stripCurrencySymbol", "parseDecimal"],
      "required": true
    },
    "occurredAt": {
      "selectorType": "css",
      "selectorExpression": "td.transaction-date",
      "postProcessing": ["parseDate:DD/MM/YYYY HH:mm"],
      "required": true
    },
    "merchant": {
      "selectorType": "regex",
      "selectorExpression": "Merchant:\\s*(.+)",
      "extractionGroup": 1,
      "postProcessing": ["trimWhitespace"],
      "required": true
    },
    "paymentMethod": {
      "selectorType": "regex",
      "selectorExpression": "Card ending in (\\d{4})",
      "extractionGroup": 1,
      "postProcessing": ["trimWhitespace"],
      "required": false
    },
    "eventTypeHint": {
      "selectorType": "static",
      "staticValue": "Expense"
    }
  }
}
```

**Note on `eventTypeHint` as `static`:** Many senders exclusively send one kind of notification (e.g., a specific bank alert template is always a debit/expense notification) — the DSL supports a `static` selector type for fields with a fixed, sender-known value, avoiding unnecessary extraction logic for fields that don't vary per message for that sender.

---

## 5. Template Matching & Execution

### 5.1 Matching

- `senderPattern` supports exact address match, domain match, and wildcard subdomain match (as in the example above).
- When multiple `SenderTemplate` versions exist for the same `senderPattern` (during a transition period, e.g., a bank changing its email format), only the row with `active = true` (per `08_database_design.md` Section 9.1) is used for new extractions — **exactly one active version per sender pattern at a time**, enforced at the application layer during template publishing (Section 6).

### 5.2 Execution & Partial-Match Handling

The Extraction Rule Executor runs every field's rule independently and classifies the overall result:

| Outcome | Condition | Result |
|---|---|---|
| **Full match** | All fields (required and optional) successfully extracted | `CandidateFinancialEvent` with `confidence = 1.0`, `method = template` |
| **Partial match, required fields present** | All `required` fields extracted; some optional fields (e.g., `paymentMethod`) missing | `CandidateFinancialEvent` with a slightly reduced confidence (default 0.95, configurable) reflecting the missing optional context, `method = template` — **still bypasses AI entirely** |
| **Template execution failure** | Any `required` field fails to extract (selector matches nothing, or `postProcessing` fails, e.g., `parseDecimal` on non-numeric content) | Triggers AI fallback (Section 7); logged as `TemplateExtractionFailed` |

**Why a reduced-but-not-zero confidence for partial matches (not full 1.0):** A template that reliably fails to find `paymentMethod` for a specific sender's format variant is genuinely providing slightly less complete information than a fully-matched extraction, and this should be reflected in the aggregate confidence used downstream (`04_domain_model.md`'s Confidence value object) — even though the reduction is deliberately small, since a missing *optional* field is a much weaker signal of unreliability than a missing *required* one (this mirrors, at the deterministic-parser level, the same weighted-importance philosophy `11_ai_blueprint.md` Section 6.2 applies to AI-derived confidence).

---

## 6. SenderTemplate Authoring & Versioning Workflow

### 6.1 Authoring Process

1. A candidate sender is identified for template creation — either proactively (a high-volume known bank/e-wallet targeted before launch, per `02_product_requirement_document.md` Section 6.1's MVP known-sender curation) or reactively (Prompt Monitoring/Parser Monitoring data from `11_ai_blueprint.md`/Section 8 shows a sender generating high AI-extraction volume, indicating it's a good template-creation candidate to reduce cost).
2. A sample set of real (anonymized/redacted for template-authoring purposes) emails from that sender is collected.
3. Rules are authored against the DSL (Section 4), covering the required fields at minimum.
4. The candidate template is validated against the sample set (an automated template-testing harness, conceptually similar to `11_ai_blueprint.md` Section 4.3's prompt evaluation process, but for deterministic rules — every sample email must produce the expected extraction, checked via exact-match assertion, not confidence thresholds, since deterministic extraction has no ambiguity to tolerate).
5. On passing validation, the template is published as a new `SenderTemplate` row (Section 6.2).

### 6.2 Versioning & Publishing

- Publishing a new version for an existing `senderPattern`: the new row is inserted with `active = true`, and the previously active row for the same `senderPattern` is atomically set to `active = false` in the same transaction — a template swap is never a window where zero or multiple templates are simultaneously active for one sender.
- **Old (deactivated) versions are never deleted** — per SRS-FR-AI-04's attribution philosophy applied symmetrically here: every `financial_events.provenance_template_version` reference must remain resolvable indefinitely for audit purposes.
- A rollback (reactivating a previous version) follows the identical atomic-swap mechanism — no special-cased "rollback" operation exists; it is simply publishing a new "version" that happens to have prior content.

### 6.3 Change Management Discipline

Mirroring `11_ai_blueprint.md` Section 4.3's regression-gated promotion for prompts, template publishing requires the automated validation step (Section 6.1, Step 4) to pass before activation — a template change is never manually pushed live without passing its own sample-set regression suite. This discipline exists specifically because a broken template silently degrades to AI fallback (graceful, but costly, per Section 7) or, worse, silently produces a **partial-match false-required-pass** (e.g., a `postProcessing` bug that parses a wrong but plausible-looking amount) — the sample-set validation is the primary defense against this second, more dangerous failure mode.

---

## 7. Template-to-AI-Fallback Mechanism

Per `07_sequence_diagrams.md` Diagram 3/4 and `06_architecture.md`'s Understanding Context policy chain:

1. A `TemplateExtractionFailed` domain event (per `04_domain_model.md` Section 7.3) is emitted whenever Section 5.2's "template execution failure" outcome occurs.
2. The `understanding` module's policy handler (`onTemplateExtractionFailed`, per `10_backend_blueprint.md` Section 7's naming convention) issues the `InvokeAIExtractor` command for the same `IngestedMessage`, transparently — **the fallback is invisible to every downstream consumer**, since both paths converge on the identical `CandidateFinancialEvent` shape (`04_domain_model.md` Section 7.2's symmetry design).
3. The failure is simultaneously recorded against that specific `SenderTemplate`'s rolling failure-rate metric (Section 8), independent of whether the AI fallback itself succeeds — the template's reliability is tracked regardless of the fallback's outcome, since a template that's failing is a maintenance signal regardless of how gracefully the system compensates.

---

## 8. Parser Monitoring: Failure-Rate-Spike Detection (H3 Early-Warning)

While `06_architecture.md` Section 6.5 resolves H3's **cost/throughput** risk at the queue-concurrency level, this section specifies the **detection** half: how the platform notices a template has started failing, ideally before the cost/throughput symptom becomes severe.

### 8.1 Rolling Failure-Rate Computation

- For each active `SenderTemplate`, a rolling window (default: trailing 1 hour, trailing 24 hours — both tracked, for fast and slow-onset detection respectively) computes `failureRate = TemplateExtractionFailed count / total execution attempts` for that template.
- Computed by a scheduled aggregation job reading from the `extraction_results` table (`08_database_design.md` Section 9.2), populating `administration.parser_metrics` (Section 13.2 of the same document).

### 8.2 Spike Detection & Alerting

- A **baseline failure rate** is established per template (typically near-zero for a healthy template, since template execution is deterministic — any non-trivial sustained failure rate is inherently anomalous, unlike AI extraction where some baseline "failure"/low-confidence rate is expected).
- An alert fires (Prometheus/Grafana, per `17_observability.md`) when a template's trailing-1-hour failure rate exceeds a low absolute threshold (default 5%, configurable — deliberately low, since **any** sustained template failure rate above near-zero indicates the sender changed their format and every failing message is now incurring unnecessary AI cost/latency, per Section 7's fallback).
- The alert payload includes the specific `senderPattern`, `templateVersion`, current failure rate, and a direct link to recent failing sample messages (via `ingested_messages`/`extraction_results` correlation) — giving the on-call/template-maintenance engineer immediate, actionable context to begin authoring a fixed template version (Section 6), not just a generic "something is wrong" signal.

### 8.3 Relationship to H3's Queue-Level Mitigation

Section 8.2's alert is explicitly the **trigger condition** referenced in `06_architecture.md` Section 6.5: a spiking `understanding:ai` queue depth **correlated with** a specific template's failure-rate spike is the platform's complete H3 signal — the queue metric shows *that* something is overloading the AI path; this document's failure-rate metric shows *which* template broke and *why*, closing the loop from symptom to root cause.

---

## 9. Detailed Explanation: Why a Declarative DSL, Not Per-Sender Go Code

An alternative design would implement each sender's extraction logic as a hand-written Go function, compiled into the binary. This document explicitly rejects that approach because:

1. **Deployment velocity:** Sender email formats change on their own schedule, unrelated to the platform's release cycle. A `jsonb`-stored, data-driven template can be published (Section 6.2) via an internal admin action or a lightweight CI pipeline **without a full application redeploy** — critical for closing the H3 failure loop quickly (a broken template directly costs money via AI fallback for every affected message until fixed).
2. **Safety:** A closed, enumerated `postProcessing` transformation set (Section 4.2) and declarative selectors mean a template author (who may be a less-senior engineer or, eventually, an ops/content specialist) cannot introduce arbitrary code-execution risk or a crash-causing bug into the production binary — the executor's own code (which *is* compiled Go, reviewed through normal engineering process) is the only place actual logic executes; templates are pure data.
3. **Testability symmetry with the Sample-Set validation process** (Section 6.1): a declarative rule set can be mechanically validated against a sample-set fixture without needing to compile/deploy a test build — this makes the regression-gated publishing workflow (Section 6.3) fast enough to be a routine, low-friction operation rather than a heavyweight engineering task.

---

## 10. Component Responsibilities

| Component | Responsibility |
|---|---|
| Template Matcher | Resolve the correct active `SenderTemplate` for an ingested message's sender |
| Extraction Rule Executor | Deterministically apply DSL rules; classify full/partial/failed outcomes |
| Template Authoring/Validation Harness | Enforce regression-gated publishing (Section 6.3) against sample-set fixtures |
| `onTemplateExtractionFailed` policy handler | Transparent, symmetric hand-off to AI Extractor |
| Parser Monitoring aggregation job | Rolling failure-rate computation per template, feeding alerting and the Admin dashboard |

---

## 11. Trade-offs

| Decision | Trade-off |
|---|---|
| Declarative DSL (data) over per-sender compiled code | Somewhat less expressive than arbitrary code for genuinely unusual sender formats (mitigated by the DSL's `regex` selector type, which covers the vast majority of edge cases; a sender too irregular even for regex simply falls back to AI extraction, which is an acceptable, intended safety valve, not a gap). |
| Reduced-but-nonzero confidence for partial matches | A small amount of confidence-tuning complexity (Section 5.2's configurable reduction value) vs. a simpler binary full-match/failure model, in exchange for more accurate downstream gating behavior when optional fields are legitimately, harmlessly absent. |
| Low absolute failure-rate alert threshold (5%) | Higher alert sensitivity (more alerts to triage) than a percentile/statistical-anomaly-based threshold would produce, in exchange for simplicity and fast reaction time — justified because, unlike AI extraction, template failure has near-zero acceptable baseline, making a simple absolute threshold appropriate rather than under-engineered. |

## 12. Advantages
- The DSL's data-driven nature directly enables the sub-24-hour-fix turnaround needed to make the H3 mitigation loop (detect → diagnose → fix → republish) actually fast in practice, not just architecturally possible.
- Symmetric `CandidateFinancialEvent` output with the AI path (Section 7) means the fallback truly is invisible downstream — no special-casing anywhere else in the system for "was this template or AI extracted," except where explicitly needed (provenance, monitoring).
- The regression-gated template publishing workflow (Section 6.3) directly prevents the higher-risk failure mode of a subtly-wrong template silently corrupting ledger data, not just the more obvious "template stops matching at all" failure mode.

## 13. Disadvantages
- Building and maintaining the sample-set validation harness (Section 6.1) is nontrivial upfront tooling investment, required before the first template can be safely published — an accepted cost given the correctness stakes.
- The declarative DSL's closed `postProcessing` transformation set (Section 4.2) means a genuinely novel data-cleaning need (e.g., an unusual date format Google/locale libraries don't already support via `parseDate`) requires an engineering change to add a new transformation primitive, rather than being solvable purely within template content — a deliberate safety/flexibility trade-off (Section 9).

## 14. Future Improvements
- Explore semi-automated template *drafting* assistance (e.g., using the AI Extractor's own successful extractions from a new unknown sender as a starting-point suggestion for a human template author) once template-creation volume becomes a scaling bottleneck for the content/ops team — this is a deliberate, natural synergy between the two extraction paths worth exploiting later, not at MVP.
- Extend the DSL with conditional logic (e.g., "if field X is present, use rule A for amount, else rule B") if real-world sender format variance within a single `senderPattern` proves common enough to justify the added DSL complexity — currently handled by treating format variants as distinct template versions instead.
- Apply the same sample-set regression-testing discipline to build an ongoing "sender format drift" canary: periodically re-running active templates against freshly-sampled recent messages (even when no failure has been detected yet) to catch slow, partial format drift before it crosses the failure-rate alert threshold.

## 15. Best Practices
- No `SenderTemplate` may be published without passing its sample-set validation (Section 6.1, Step 4) — this must be a CI-enforced gate for the template-publishing pipeline, not a manual checklist item trusted to human diligence.
- Every `postProcessing` transformation added to the enumerated set (Section 4.2) must itself be unit-tested against edge cases (locale variants, malformed input) before being made available to template authors, since a bug in a shared transformation affects every template using it.
- Parser Monitoring's failure-rate threshold (Section 8.2) must be reviewed whenever the known-sender template library grows significantly, to ensure alert volume remains actionable rather than becoming noise that gets ignored.

## 16. Production Considerations
- The rolling failure-rate aggregation job (Section 8.1) must itself be monitored for liveness — an aggregation job silently failing would blind Parser Monitoring exactly when it's needed most (during an actual sender-format-change incident), mirroring the same "monitor the monitor" discipline established for the poll-fallback safety net in `12_gmail_connector.md` Section 17.
- Template authoring/publishing access should be scoped to a specific internal role (not general engineering write access to production configuration), with the publish action itself audit-logged (`administration.audit_log`, per `08_database_design.md` Section 13.1) given its direct, immediate production impact on ledger extraction accuracy.
- The sample-set fixtures used for validation (Section 6.1) must be handled per the same PII-minimization principles as `11_ai_blueprint.md` Section 9, since they contain real (if redacted) sender email content and are a persistent artifact, unlike a single extraction request.

---

**Next Document:** `14_workflow_engine.md` — specifies n8n's precisely-bounded role in the platform (per `06_architecture.md` Section 11's explicit constraint that n8n never touches core financial processing), covering which operational/administrative automations it owns and the integration boundary with the core Go backend.
