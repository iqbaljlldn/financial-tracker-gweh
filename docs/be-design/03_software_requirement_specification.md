# Software Requirement Specification (SRS)
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`, `02_product_requirement_document.md`
**Feeds Into:** `04_domain_model.md`, `05_event_storming.md`, `06_architecture.md`, `08_database_design.md`, `09_api_specification.md`

---

## 1. Purpose

This document converts the product requirements in `02_product_requirement_document.md` into **engineering-grade, testable, measurable specifications**. Where the PRD states *what* the product must do for the user, the SRS states *how the system must behave* — in terms of inputs, outputs, processing rules, constraints, interfaces, and quality attributes — with enough precision that:

1. The Domain Model (`04_domain_model.md`) can be derived directly from the entities and behaviors implied here.
2. The Architecture (`06_architecture.md`) can be justified against explicit NFR targets, not vague adjectives like "scalable."
3. QA (`18_testing_strategy.md`) can write test plans without needing product clarification.

This SRS follows the spirit of IEEE 830 / ISO/IEC/IEEE 29148 structuring (functional requirements, external interface requirements, non-functional requirements, constraints, system boundaries) adapted for a modular-monolith, event-driven, AI-augmented backend system.

## 2. Scope

### 2.1 In Scope
- Formalized functional requirements (`SRS-FR-*`) mapped 1:1 or 1:many from `FR-*` PRD IDs.
- External interface requirements (Gmail API, WhatsApp Business API, LLM Provider API).
- Non-functional requirements with **measurable targets** (latency, throughput, availability, consistency).
- System constraints (regulatory, technical, operational).
- System boundary definition (what is inside vs. outside the platform's responsibility).
- Data retention and lifecycle requirements at the specification level (implementation detail deferred to `08_database_design.md`).

### 2.2 Out of Scope
- Entity/relationship modeling → `04_domain_model.md`
- Concrete technology selection rationale → `06_architecture.md`
- Schema DDL → `08_database_design.md`
- API payload contracts → `09_api_specification.md`

---

## 3. Architecture (Specification-Level System Boundary)

```
┌───────────────────────────────────────────────────────────────────────┐
│                         SYSTEM BOUNDARY                                 │
│                                                                          │
│   ┌────────────┐   ┌────────────┐   ┌────────────┐   ┌───────────────┐│
│   │  Identity  │   │  Ingestion │   │ Understanding│  │  Financial     ││
│   │  & Access  │──▶│  (Gmail)   │──▶│  (Parser/AI) │─▶│  Core (Ledger, ││
│   │            │   │            │   │              │  │  Analytics,    ││
│   │            │   │            │   │              │  │  Budget)       ││
│   └────────────┘   └────────────┘   └────────────┘   └───────┬───────┘│
│                                                                 │        │
│                                                                 ▼        │
│                                                        ┌───────────────┐│
│                                                        │  Engagement   ││
│                                                        │  (WhatsApp)    ││
│                                                        └───────────────┘│
│                                                                          │
│   ┌──────────────────────────────────────────────────────────────────┐│
│   │                       Administration (Cross-Cutting)               ││
│   └──────────────────────────────────────────────────────────────────┘│
└───────────────────────────────────────────────────────────────────────┘
          ▲                    ▲                              ▲
          │                    │                              │
   ┌─────────────┐     ┌──────────────┐              ┌────────────────┐
   │ Google OAuth │     │  Gmail API    │              │ WhatsApp        │
   │  (external)  │     │  (external)   │              │ Business API    │
   └─────────────┘     └──────────────┘              │ (external)      │
                                                        └────────────────┘
                              ┌──────────────────┐
                              │  LLM Provider API │
                              │  (external, OpenAI│
                              │  compatible)       │
                              └──────────────────┘
```

**System boundary statement:** The platform owns identity/session management, connector orchestration, extraction/normalization, the financial ledger, analytics/budget computation, and notification composition. It does **not** own: Google's mail infrastructure, WhatsApp's message delivery infrastructure, or the LLM provider's inference infrastructure — these are treated as external dependencies with defined interface contracts and failure-handling requirements (Section 6).

---

## 4. Functional Requirements (Engineering-Grade)

Each `SRS-FR-*` requirement traces to one or more `FR-*` PRD IDs. Format: **Requirement → Inputs → Processing Rule → Outputs → Error Handling.**

### 4.1 Identity & Access (traces to FR-AUTH-01, FR-AUTH-02)

**SRS-FR-AUTH-01: OAuth Authorization Code Flow with PKCE**
- **Input:** User-initiated sign-in request.
- **Processing Rule:** System redirects to Google's OAuth 2.0 endpoint with `scope=gmail.readonly openid email profile`, `access_type=offline`, `prompt=consent` (to guarantee refresh token issuance), and a PKCE code challenge.
- **Output:** On callback, system exchanges the authorization code for access + refresh tokens; tokens are encrypted at rest (Section 7.3) and associated with a `user` and `gmail_account` record.
- **Error Handling:** Invalid/expired authorization code → user redirected to retry with an explanatory message; no partial user/account record persists.

**SRS-FR-AUTH-02: Token Lifecycle Management**
- **Input:** Scheduled token-refresh job; API-call-triggered refresh-on-401.
- **Processing Rule:** Access tokens refreshed proactively (before expiry, using stored `expires_at`) and reactively (on 401 from Gmail API). Refresh failures classified as `invalid_grant` (revoked — terminal) vs. transient (retry).
- **Output:** Updated encrypted token record, or `connector_status = disconnected` on terminal failure.
- **Error Handling:** Terminal failure triggers a user-facing reconnection notification (WhatsApp + in-app) and halts further sync attempts for that connector until reconnection.

### 4.2 Gmail Connector (traces to FR-GMAIL-01..04)

**SRS-FR-GMAIL-01: Initial Full Sync**
- **Input:** Newly authorized `gmail_account`.
- **Processing Rule:** Query Gmail `messages.list` with a configurable lookback window (default 90 days) and a query filter narrowing to likely transactional mail (sender heuristics, keyword heuristics — see `13_email_parser.md`). Enqueue each candidate message as an `EmailIngested` event (Inbox pattern).
- **Output:** A bounded set of ingestion jobs; a `sync_state` record capturing progress and the final `historyId` at completion.
- **Error Handling:** Partial failure resumes from last successfully processed page token; full sync is idempotent — re-running produces no duplicate ingestion jobs (dedup by Gmail `message_id`).

**SRS-FR-GMAIL-02: Push-Based Watch**
- **Input:** Gmail `users.watch` registration against a configured Pub/Sub topic.
- **Processing Rule:** System registers/renews the watch before its 7-day expiry (renewal scheduled at ≤ 6 days to provide margin). On receiving a Pub/Sub push, the system triggers an incremental History Sync (SRS-FR-GMAIL-03) rather than processing the push payload directly (push payloads only signal "something changed," not the content).
- **Output:** A `WatchRenewed` audit event; a triggered incremental sync job.
- **Error Handling:** Missed renewal (system downtime) is detected on next health check and triggers immediate re-registration plus a bounded catch-up full sync as a safety net.

**SRS-FR-GMAIL-03: Incremental Sync via History API**
- **Input:** Stored `historyId` from the last successful sync.
- **Processing Rule:** Call `history.list` from the stored `historyId`; process only `messagesAdded` (and optionally `labelsAdded` for read/important-flag changes, out of scope for MVP). Update `historyId` only after successful processing (at-least-once semantics upstream of idempotent downstream handling).
- **Output:** New `EmailIngested` events for each newly discovered message.
- **Error Handling:** `404 historyId not found` (expired, Gmail retains ~7-30 days of history) triggers fallback to a bounded re-sync window (e.g., last 3 days) rather than a full 90-day re-scan, logged as a `HistorySyncFallback` administrative event.

**SRS-FR-GMAIL-04: Sender Identification**
- **Input:** Ingested email (headers + body).
- **Processing Rule:** Match sender domain/address against the `known_sender_registry`. Additionally apply a transactional-content classifier (keyword/heuristic, extensible to ML classifier later) to filter non-financial mail before it reaches extraction.
- **Output:** Routing decision: `template_parser` | `ai_extractor` | `discarded`.
- **Error Handling:** Ambiguous classification (neither clearly transactional nor clearly not) defaults to `ai_extractor` routing rather than silent discard, favoring recall over precision at this stage (false negatives — missed transactions — are worse than a wasted AI call).

### 4.3 Email Processing (traces to FR-PARSE-01..03)

**SRS-FR-PARSE-01: Deterministic Template/Regex Parsing**
- **Input:** Email routed to `template_parser`, matched `sender_template` version.
- **Processing Rule:** Apply the versioned extraction rules (DOM/regex selectors) to produce candidate fields: merchant, amount, currency, date, payment_method, event_type_hint.
- **Output:** A `CandidateFinancialEvent` with `extraction_method = template`, `template_version`, and a deterministic confidence score (default 1.0 for fully matched fields, reduced per missing/ambiguous field).
- **Error Handling:** Any required field failing to match (e.g., amount not found) triggers automatic fallback to `ai_extractor` for that email, with the template failure logged and counted toward that template's failure-rate metric (Parser Monitoring).

**SRS-FR-PARSE-02: Duplicate Detection**
- **Input:** Any `CandidateFinancialEvent` prior to persistence.
- **Processing Rule:** Primary dedup key = `(gmail_account_id, gmail_message_id)` — enforced via a unique constraint (Inbox pattern / idempotency key). Secondary fuzzy dedup (amount + merchant + timestamp window ± N minutes) applied only when multi-source ingestion is active (Phase 2+).
- **Output:** Either a newly persisted event or a no-op (already processed) result.
- **Error Handling:** Constraint violation on insert is treated as success-path idempotency (not an error) — the job completes without creating a duplicate.

**SRS-FR-PARSE-03: Retry with Backoff**
- **Input:** Any transient processing failure (timeouts, rate limits, provider 5xx).
- **Processing Rule:** Exponential backoff retry (base delay, multiplier, jitter, max attempts — configurable; defaults specified in `10_backend_blueprint.md`). Distinguish retryable (network/5xx/429) vs. non-retryable (4xx validation) errors.
- **Output:** Successful processing, or dead-letter queue placement after max attempts.
- **Error Handling:** Dead-lettered jobs are visible in Administration → Parser Monitoring with full failure context (last error, attempt count, original payload reference).

### 4.4 AI Engine (traces to FR-AI-01..04)

**SRS-FR-AI-01: Structured Extraction**
- **Input:** Email content (sanitized/truncated per token budget) routed to `ai_extractor`.
- **Processing Rule:** Invoke the LLM with a versioned prompt template requesting a strictly structured JSON response (merchant, amount, currency, date, category_hint, payment_method, per-field confidence). Response is schema-validated before acceptance.
- **Output:** A `CandidateFinancialEvent` with `extraction_method = ai`, `prompt_version`, `model_identifier`, per-field confidence, and an aggregate confidence score (weighted per `11_ai_blueprint.md`).
- **Error Handling:** Schema-invalid LLM response triggers one automatic re-prompt (with a stricter instruction); repeated failure routes the email to the manual review queue rather than the retry/dead-letter path (this is a content-understanding failure, not a transient error).

**SRS-FR-AI-02: AI Categorization**
- **Input:** Any finalized or near-finalized `CandidateFinancialEvent` (regardless of extraction method).
- **Processing Rule:** Categorization considers: (a) user's historical manual corrections for the same merchant (highest priority), (b) global merchant-category mapping learned across users (anonymized/aggregated), (c) LLM-based semantic categorization as fallback.
- **Output:** `category`, `category_confidence`, `category_source` (`user_history` | `global_mapping` | `ai_inference`).
- **Error Handling:** No confident category available → category = `Uncategorized`, surfaced in analytics as its own bucket rather than blocking event finalization.

**SRS-FR-AI-03: Confidence Gating**
- **Input:** Aggregate confidence score of a `CandidateFinancialEvent`.
- **Processing Rule:** Threshold-based routing (configurable, default 0.70): `≥ threshold → auto-finalize into ledger`; `< threshold → pending_review state`, surfaced to the user for confirmation.
- **Output:** `FinancialEvent.status ∈ {finalized, pending_review}`.
- **Error Handling:** Events remaining in `pending_review` beyond a configurable SLA (default 7 days) trigger an escalation notification and are excluded from analytics/budget computation until resolved (Section 4.7 clarifies analytics scope).

**SRS-FR-AI-04: Prompt Versioning & Attribution**
- **Input:** Every AI call.
- **Processing Rule:** Prompt templates are stored as versioned, immutable artifacts (content-addressed or semantically versioned). Every AI-derived record stores the exact `prompt_version` and `model_identifier` used.
- **Output:** Immutable attribution on every AI-derived `FinancialEvent`, enabling retrospective analysis by prompt version (Prompt Monitoring, `11_ai_blueprint.md`).
- **Error Handling:** N/A (this is an attribution/audit requirement, not a failure-handling one).

### 4.5 Ledger (traces to FR-LEDGER-01..03)

**SRS-FR-LEDGER-01: FinancialEvent Persistence**
- **Input:** A validated `CandidateFinancialEvent` (post confidence-gating).
- **Processing Rule:** Persist as an immutable-by-default `FinancialEvent` (append-only preferred; corrections are new versions/overrides, not destructive updates — see `04_domain_model.md` for the versioning strategy). Full source lineage retained: `connector_id`, `source_message_ref`, `extraction_method`, `template_version`/`prompt_version`, `confidence`.
- **Output:** Persisted record, queryable by the Analytics/Budget/Notification consumers.
- **Error Handling:** Any persistence failure re-queues the finalization step (idempotent — dedup key from SRS-FR-PARSE-02 still applies).

**SRS-FR-LEDGER-02: Manual Override**
- **Input:** User-submitted field correction via API.
- **Processing Rule:** Correction is stored as an override layer (original extracted values remain intact for audit) with actor, timestamp, previous value, new value. Overridden fields are excluded from future automated re-processing overwrite.
- **Output:** Updated effective `FinancialEvent` view (override-applied), full audit trail preserved.
- **Error Handling:** Concurrent override conflicts (rare, multi-device) resolved via last-write-wins at the field level, with both attempts logged.

### 4.6 Analytics (traces to FR-ANALYTICS-01..04)

**SRS-FR-ANALYTICS-01..04: Aggregate Computation**
- **Input:** Date range, user (and optionally `gmail_account` filter), aggregation dimension (category/merchant/day/month).
- **Processing Rule:** All analytics queries operate **exclusively** over `FinancialEvent` records with `status = finalized`. `pending_review` and soft-deleted records are excluded by default (with an explicit opt-in parameter for "include pending" views, used only by the review-queue UI, not general analytics).
- **Output:** Aggregated, correctly-rounded monetary sums (currency-aware; see Section 6.4 on multi-currency handling), returned via read-optimized query paths (CQRS read models — see `06_architecture.md`).
- **Error Handling:** Empty result sets return a well-defined empty structure (zeroed aggregates), never an error, to keep client rendering simple.

### 4.7 Budget Engine (traces to FR-BUDGET-01..03)

**SRS-FR-BUDGET-01/02: Budget Definition & Overspending Detection**
- **Input:** User-defined budget (scope: global or per-category, period: monthly), real-time finalized event stream.
- **Processing Rule:** On every new finalized `Expense`-type event, recompute cumulative spend against the relevant active budget period. Threshold crossings (default 80%, 100%, configurable) trigger a **single** notification per threshold per period (idempotent alerting — re-crossing due to a later correction does not re-fire an already-fired threshold).
- **Output:** `BudgetAlertTriggered` domain event consumed by the Notification module.
- **Error Handling:** Budget recomputation is designed to be replay-safe (can be recomputed from the event stream at any time without double-alerting), supporting the Outbox/Event-Driven architecture in `06_architecture.md`.

### 4.8 Notification (traces to FR-NOTIF-01..05)

**SRS-FR-NOTIF-01..03: Scheduled Summaries**
- **Input:** Scheduled job (per user-configured time; default local-timezone-aware daily/weekly/monthly triggers).
- **Processing Rule:** Compose summary from Analytics read models (never recompute raw aggregation logic in the Notification module — strict separation of concerns per `06_architecture.md`). Render via a templating system supporting WhatsApp message format constraints (character limits, template approval requirements).
- **Output:** Delivered WhatsApp message via WhatsApp Business API; delivery status tracked.
- **Error Handling:** See SRS-FR-NOTIF-05.

**SRS-FR-NOTIF-05: Delivery Reliability**
- **Input:** Notification composition result.
- **Processing Rule:** Delivery attempted via WhatsApp Business API; transient failures retried with backoff (shared retry infrastructure with Section 4.3); permanent failures (invalid number, user blocked business account) mark the notification channel as `degraded` for that user and surface in Administration.
- **Output:** `delivery_status ∈ {delivered, failed, retrying}` per notification instance, stored for audit and troubleshooting.
- **Error Handling:** N/A beyond above — this requirement *is* the error-handling specification for notification delivery.

### 4.9 Administration (traces to FR-ADMIN-01..05)

**SRS-FR-ADMIN-01..05:** Administration requirements are primarily **observability and audit** requirements rather than transactional processing rules; they are fully specified in `17_observability.md` (metrics/dashboards) and `16_security.md` (audit log integrity, retention, and access control). This SRS establishes only the data-capture obligation: every module in Sections 4.1–4.8 **must emit** structured events/metrics sufficient to satisfy FR-ADMIN-01..05 without requiring additional instrumentation retrofits later.

---

## 5. External Interface Requirements

| Interface | Direction | Protocol | Requirement Summary |
|---|---|---|---|
| Google OAuth 2.0 | Outbound | HTTPS/REST | PKCE-enabled Authorization Code flow; scopes limited to `gmail.readonly`, `openid`, `email`, `profile` |
| Gmail API (`messages`, `history`, `watch`) | Outbound | HTTPS/REST | Rate-limit aware (Gmail per-user quota); exponential backoff on 429/5xx |
| Google Cloud Pub/Sub | Inbound | Push (HTTPS webhook) | Signature/JWT verification on every push payload before processing |
| LLM Provider (OpenAI-compatible) | Outbound | HTTPS/REST | Provider-agnostic client abstraction (interface, not concrete SDK, at the domain boundary); timeout + retry policy; PII-minimization in prompts where feasible |
| WhatsApp Business API | Outbound | HTTPS/REST | Pre-approved message templates for proactive (non-session) messages; delivery webhook consumption for status updates |
| Platform Public API | Inbound | HTTPS/REST (OpenAPI-documented) | Consumed by first-party web/mobile clients; authenticated via platform session tokens |

Full contract-level detail (payload schemas, status codes) is specified in `09_api_specification.md` for the platform's own API, and in `12_gmail_connector.md` / `15_notification_system.md` for the external integrations.

---

## 6. Non-Functional Requirements (Measurable)

### 6.1 Performance & Latency

| Requirement | Target |
|---|---|
| New email → `FinancialEvent` finalized (known sender, template path) | P95 ≤ 2 minutes from Gmail delivery |
| New email → `FinancialEvent` finalized (AI extraction path) | P95 ≤ 5 minutes from Gmail delivery |
| Analytics query response time | P95 ≤ 500ms for standard date-range aggregations |
| WhatsApp notification delivery (post-composition) | P95 ≤ 30 seconds to provider acceptance |

### 6.2 Availability & Reliability

| Requirement | Target |
|---|---|
| Core API availability | ≥ 99.9% monthly uptime |
| Ingestion pipeline durability | Zero data loss for accepted Gmail push notifications (durable queue, at-least-once delivery to processing workers) |
| Gmail Watch renewal reliability | ≥ 99.99% successful renewal before expiry (7-day window, renew at ≤ 6 days) |

### 6.3 Consistency & Correctness

- **Exactly-once effective processing:** While transport-level delivery is at-least-once (Gmail push, queue redelivery), the system **must** guarantee exactly-once *effect* on the ledger via idempotency keys (SRS-FR-PARSE-02) and the Outbox/Inbox pattern (`06_architecture.md`).
- **No lost updates on manual override:** Concurrent edits must not silently discard a user's manual correction (SRS-FR-LEDGER-02).
- **Analytics correctness:** Aggregates must always be computable, deterministically, from the finalized event stream — no analytics-only derived state may exist that cannot be reconstructed from the ledger (supports disaster recovery and CQRS read-model rebuilding).

### 6.4 Multi-Currency Handling

- Every `FinancialEvent` stores its native currency and amount (never force-converted at ingestion).
- Analytics aggregation across mixed currencies requires an explicit conversion strategy (documented in `04_domain_model.md`/`08_database_design.md`); the SRS mandates that **raw amounts are never lossy-converted or overwritten** — conversion, if performed, produces a derived, clearly-labeled secondary value.

### 6.5 Scalability

| Dimension | Target Design Point |
|---|---|
| Linked mailboxes | Design for 100,000+ linked Gmail accounts without architectural change (horizontal worker scaling) |
| Ingestion throughput | Design for sustained 500 emails/sec platform-wide processing capacity, burstable |
| AI extraction throughput | Must be decoupled (async, queue-based) from ingestion rate to allow independent scaling and provider rate-limit management |

### 6.6 Security (Specification-Level; full detail in `16_security.md`)

- OAuth tokens encrypted at rest (envelope encryption, KMS-backed).
- All external API traffic over TLS 1.2+.
- Principle of least privilege: `gmail.readonly` scope only — the system never requests send/modify/delete Gmail scopes.
- All administrative access to raw email content or tokens is itself audit-logged (meta-audit requirement).

### 6.7 Compliance & Data Lifecycle

- User-initiated data export must be supported (GDPR Art. 20 portability intent).
- User-initiated account/data deletion must propagate to: `FinancialEvent` records, raw email content caches (if any — see retention policy below), OAuth tokens (immediately revoked at Google), and derived analytics.
- **Raw email content retention:** Raw email bodies are retained only as long as necessary for extraction/debugging (default configurable window, e.g., 30 days), after which only the derived `FinancialEvent` and lineage metadata are retained — minimizing sensitive data footprint over time.

### 6.8 Maintainability & Modularity

- Every module boundary identified in Section 3 must be independently testable and independently deployable-in-principle (even if deployed as a single artifact at launch), per the Vision's "modular monolith, microservice-ready" principle.
- No module may reach into another module's persistence schema directly; all cross-module interaction occurs via defined interfaces/domain events (enforced architecturally in `06_architecture.md`).

---

## 7. Constraints

| Constraint | Description |
|---|---|
| Gmail API Quotas | Google enforces per-user and per-project quota limits; system design must operate within these without requiring quota increases as a launch blocker. |
| Gmail Watch Expiry | Hard 7-day maximum lifetime per Gmail API — renewal cannot be architected around; it must be scheduled and monitored. |
| WhatsApp Business API Policy | Proactive (business-initiated) messages outside a 24-hour customer-service window require pre-approved message templates — notification content design must account for this. |
| LLM Provider Rate/Cost Limits | AI extraction/categorization calls are subject to provider rate limits and per-token cost; architecture must treat AI calls as a scarce, budgeted resource (informs the deterministic-first strategy from the Vision). |
| Regulatory | GDPR (data subject rights) at minimum; architecture should not preclude future compliance with region-specific financial data regulations (e.g., data residency) even if not required at MVP. |

---

## 8. Detailed Explanation: Why Confidence Gating Is a Hard Specification, Not a Feature Toggle

A recurring temptation in AI-augmented systems is to treat confidence thresholds as a tunable "nice-to-have" parameter that can be defaulted to always-accept for simplicity. This SRS explicitly rejects that framing for this domain:

- The ledger is a **financial system of record**. An incorrectly auto-finalized event (wrong amount, wrong merchant) directly corrupts analytics, budget alerts, and — most critically — user trust in a product whose entire value proposition is "you don't have to check this yourself."
- Confidence gating (SRS-FR-AI-03) is therefore specified as a **mandatory processing stage** between extraction and persistence, not an optional post-hoc filter. Every extraction path — template or AI — produces a confidence score and passes through this gate identically, which also keeps the two extraction paths structurally symmetric (important for `04_domain_model.md`, where both must conform to the same `CandidateFinancialEvent` shape).

## 9. Component Responsibilities (Specification-Level)

| Component | Responsibility Boundary |
|---|---|
| Identity & Access | Owns OAuth lifecycle and platform session issuance; owns nothing about email content |
| Gmail Connector | Owns mailbox synchronization state (`historyId`, watch registration); emits raw ingestion events, does not interpret content |
| Email Processing | Owns routing and deterministic extraction; owns template versioning |
| AI Engine | Owns probabilistic extraction and categorization; owns prompt versioning and confidence computation |
| Ledger | Owns the canonical, immutable-by-default financial record and its override/audit trail |
| Analytics | Owns read-optimized aggregation; owns no write path to the ledger |
| Budget Engine | Owns threshold evaluation and alert-triggering logic; owns no notification delivery mechanics |
| Notification | Owns composition and delivery; owns no aggregation logic |
| Administration | Owns cross-cutting observability/audit; owns no business logic |

---

## 10. Trade-offs

| Decision | Trade-off |
|---|---|
| Exactly-once *effect* atop at-least-once *transport* | More implementation complexity (idempotency keys everywhere) in exchange for correctness guarantees appropriate to a financial ledger. |
| Strict module boundary enforcement (no cross-schema access) | Some short-term query convenience is sacrificed for long-term extractability into microservices. |
| Raw email content time-boxed retention | Slightly reduces long-term debuggability/re-processing ability, in exchange for materially reduced sensitive-data exposure surface. |
| Confidence gating as mandatory, not optional | Adds a `pending_review` UX and state to manage, in exchange for protecting ledger correctness. |

## 11. Advantages
- Every functional requirement now has explicit input/processing/output/error-handling shape, directly usable for interface design and test-case derivation.
- NFRs are measurable, enabling objective architecture review and production readiness sign-off rather than subjective judgment calls.
- The specification-level system boundary (Section 3) gives a clean seam for the Architecture document's technology mapping.

## 12. Disadvantages
- The rigor of this document requires upfront investment before any code is written — acceptable per the Vision's "never simplify the design" directive, but it does lengthen the pre-implementation phase.
- Some NFR targets (e.g., 500 emails/sec platform-wide) are design points, not launch-day requirements, and must be clearly communicated to avoid premature over-engineering at MVP — addressed via phased scaling guidance in `23_scalability.md`.

## 13. Future Improvements
- Introduce a formal fuzzy cross-source deduplication specification once multi-connector ingestion (Outlook, SMS, Bank API) is active (Section 4.3 flags this as Phase 2+).
- Extend the confidence-gating specification to support per-user-adjustable thresholds (power users may accept more automation risk than cautious users).
- Formalize a data-residency requirement set if/when the platform targets regions with strict financial data localization laws.

## 14. Best Practices
- Treat this SRS as the canonical source for acceptance-test derivation — QA should not need to re-interpret the PRD directly.
- Any deviation discovered during implementation (e.g., an NFR target proves technically infeasible) must be reflected back into this document via a tracked amendment, not silently absorbed into code.
- Keep the "Requirement → Input → Processing Rule → Output → Error Handling" format for any new functional requirement added post-MVP, to preserve consistency for future engineers.

## 15. Production Considerations
- NFR targets in Section 6 must be continuously validated in production via the observability stack (`17_observability.md`), not just verified once at launch — SLOs should be tracked as live dashboards, not static document values.
- The confidence threshold default (0.70) is a launch estimate; it must be revisited using real extraction accuracy data within the first operational month, per the Best Practices note in `02_product_requirement_document.md`.
- External dependency constraints (Section 7) should each have a corresponding monitored health check and a documented degradation behavior (e.g., what the system does if the LLM provider is down — fall back to template-only processing and queue AI-required emails for later retry, never block the entire pipeline).

---

## 16. Traceability Matrix (Summary)

| PRD Requirement | SRS Requirement(s) |
|---|---|
| FR-AUTH-01, FR-AUTH-02 | SRS-FR-AUTH-01, SRS-FR-AUTH-02 |
| FR-GMAIL-01..04 | SRS-FR-GMAIL-01..04 |
| FR-PARSE-01..03 | SRS-FR-PARSE-01..03 |
| FR-AI-01..04 | SRS-FR-AI-01..04 |
| FR-LEDGER-01..03 | SRS-FR-LEDGER-01, SRS-FR-LEDGER-02 (LEDGER-03 deferred to Phase-appropriate API spec) |
| FR-ANALYTICS-01..04 | SRS-FR-ANALYTICS-01..04 |
| FR-BUDGET-01..03 | SRS-FR-BUDGET-01/02 (BUDGET-03 specified alongside in `04_domain_model.md`) |
| FR-NOTIF-01..05 | SRS-FR-NOTIF-01..03, SRS-FR-NOTIF-05 |
| FR-ADMIN-01..05 | Deferred fully to `16_security.md` / `17_observability.md` per Section 4.9 |

---

**Next Document:** `04_domain_model.md` — formalizes the `FinancialEvent` aggregate root, its subtypes, value objects, invariants, and domain events, directly derived from the functional requirements specified here.
