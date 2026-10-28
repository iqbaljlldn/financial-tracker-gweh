# Domain Model
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`, `02_product_requirement_document.md`, `03_software_requirement_specification.md`
**Feeds Into:** `05_event_storming.md`, `06_architecture.md`, `08_database_design.md`, `09_api_specification.md`

---

## 1. Purpose

This document defines the **tactical Domain-Driven Design model** for the platform: bounded contexts, aggregates, entities, value objects, domain events, and invariants. It is the single source of truth for "what a `FinancialEvent` actually is," and it is the document every other engineering artifact — database schema, API contracts, sequence diagrams, AI extraction schema — must derive from, not the other way around.

The central architectural commitment carried from `01_product_vision.md` is formalized here in precise DDD terms: **`FinancialEvent` is the aggregate root of the Financial Core bounded context.** Everything the product brief calls "Ledger," "Transaction," "Income," "Expense," etc. is modeled as a subtype or role within this single aggregate family — never as competing, parallel root entities.

## 2. Scope

### 2.1 In Scope
- Bounded context map and context boundaries.
- Aggregate roots, entities, and value objects per context.
- The `FinancialEvent` type hierarchy and its invariants.
- Domain events emitted by each aggregate.
- The extraction lineage / provenance model (how a `FinancialEvent` remembers where it came from).
- The override/versioning model for manual corrections.
- Ubiquitous language glossary.

### 2.2 Out of Scope
- Physical database schema (tables, indexes, partitioning) → `08_database_design.md`
- Event storming workshop artifacts (hotspots, policies, external systems detail) → `05_event_storming.md`
- API request/response shapes → `09_api_specification.md`
- Concrete AI prompt/response schema → `11_ai_blueprint.md`

---

## 3. Architecture: Bounded Context Map

```
┌──────────────────────┐      ┌──────────────────────┐      ┌──────────────────────┐
│  Identity & Access     │      │   Ingestion Context    │      │  Understanding        │
│  Context               │      │   (Connector Framework)│      │  Context              │
│                        │      │                        │      │  (Parsing + AI)        │
│  - User               │      │  - GmailAccount        │      │  - CandidateEvent      │
│  - GmailAccountLink    │─────▶│  - SyncState           │─────▶│  - ExtractionResult    │
│  - OAuthToken          │      │  - WatchSubscription    │      │  - SenderTemplate      │
└──────────────────────┘      │  - IngestedMessage      │      └──────────┬───────────┘
                                └──────────────────────┘                    │
                                                                             ▼
┌──────────────────────┐      ┌──────────────────────┐      ┌──────────────────────┐
│  Engagement Context    │      │  Budget Context        │      │  Financial Core       │
│  (Notification)        │◀─────│                        │◀─────│  Context (CORE DOMAIN) │
│                        │      │  - Budget              │      │                        │
│  - NotificationJob     │      │  - BudgetPeriod         │      │  - FinancialEvent      │
│  - DeliveryRecord       │      │  - SavingsGoal          │      │    (Aggregate Root)   │
└──────────────────────┘      └──────────────────────┘      │  - CategoryAssignment  │
                                                                │  - EventOverride       │
        ▲                                                       └──────────┬───────────┘
        │                                                                    │
        │                       ┌──────────────────────┐                    │
        └───────────────────────│  Analytics Context     │◀───────────────────┘
                                 │  (Read Model / CQRS)   │
                                 │  - CashFlowView        │
                                 │  - CategoryBreakdown    │
                                 │  - MerchantRanking      │
                                 │  - TrendView            │
                                 └──────────────────────┘

┌──────────────────────────────────────────────────────────────────────────┐
│  Administration Context (Cross-Cutting / Supporting Domain)                │
│  - ConnectorHealthRecord   - ParserMetric   - PromptMetric   - AuditLog     │
└──────────────────────────────────────────────────────────────────────────┘
```

**Core Domain:** Financial Core (`FinancialEvent` and its lifecycle) — this is where competitive differentiation and business complexity live, per DDD's core-domain identification principle.

**Supporting Domains:** Ingestion, Understanding, Budget, Engagement — necessary, non-trivial, but not where the platform's unique value is architecturally concentrated.

**Generic Subdomains:** Identity & Access, Administration — solved problems, implemented with standard patterns, minimal customization expected.

---

## 4. Ubiquitous Language Glossary

| Term | Definition |
|---|---|
| **FinancialEvent** | A typed, immutable-by-default record of any financially significant occurrence observed by the platform, regardless of source. The aggregate root of the Financial Core. |
| **Connector** | A source-specific adapter (Gmail, future Outlook/SMS/etc.) that observes raw signals and produces `IngestedMessage` records. |
| **IngestedMessage** | A raw, source-specific signal (e.g., a Gmail message) that has entered the platform but has not yet been interpreted. |
| **CandidateFinancialEvent** | An interpreted-but-not-yet-finalized structure produced by either the Template Parser or the AI Extractor, carrying a confidence score. |
| **SenderTemplate** | A versioned, deterministic extraction definition for a known email sender. |
| **ExtractionResult** | The outcome of applying a `SenderTemplate` or the AI Extractor to an `IngestedMessage`, including per-field confidence. |
| **EventOverride** | A user-submitted correction to one or more fields of a finalized `FinancialEvent`, preserved alongside (not replacing) the original extracted value. |
| **CategoryAssignment** | The categorization decision applied to a `FinancialEvent`, with its own confidence and source (`user_history`, `global_mapping`, `ai_inference`, `manual`). |
| **Budget** | A user-defined spending threshold, scoped globally or per category, for a recurring period (monthly). |
| **Ledger** | The complete, queryable collection of a user's `FinancialEvent` records — not a separate entity, but the conceptual name for the aggregate collection. |

---

## 5. The Financial Core Context (Core Domain)

### 5.1 Aggregate Root: `FinancialEvent`

`FinancialEvent` is the single aggregate root representing any financially meaningful occurrence. It is designed as a **tagged union / polymorphic aggregate**: a common envelope of fields shared by all events, plus a type-specific payload determined by `event_type`.

#### 5.1.1 Common Envelope (all FinancialEvent instances share these)

| Field | Type (conceptual) | Description |
|---|---|---|
| `id` | Identifier (UUID) | Globally unique identifier |
| `user_id` | Identifier | Owning user |
| `event_type` | Enum | One of the FinancialEvent Type Taxonomy (Section 5.2) |
| `amount` | Money (value object: amount + currency) | Native amount as extracted; never force-converted |
| `occurred_at` | Timestamp | When the financial occurrence happened (not when it was ingested) |
| `merchant` | Merchant (value object) | Normalized merchant/counterparty name, raw name, and optional merchant ID |
| `payment_method` | PaymentMethod (value object) | e.g., card last-4, wallet name, bank account hint |
| `status` | Enum (`pending_review`, `finalized`, `archived`, `disputed`) | Lifecycle state |
| `confidence` | Confidence (value object, 0.0–1.0) | Aggregate confidence from extraction |
| `provenance` | Provenance (value object) | Full lineage — see Section 5.4 |
| `category_assignment` | CategoryAssignment (entity) | Current effective categorization |
| `overrides` | List<EventOverride> | Audit trail of manual corrections |
| `created_at` / `updated_at` | Timestamp | Standard bookkeeping |

#### 5.1.2 Type-Specific Payloads

Each `event_type` may carry additional, optional structured attributes beyond the common envelope. These are modeled as **type-specific value objects attached to the envelope**, not as separate tables/aggregates (this decision is elaborated in Section 5.3).

| Event Type | Additional Attributes |
|---|---|
| `Income` | `income_source` (e.g., employer, client), `is_recurring` |
| `Expense` | `expense_category_hint`, `is_recurring` |
| `Transfer` | `counterparty_account_hint`, `direction` (`outgoing`/`incoming`), `linked_transfer_event_id` (optional, if both legs are observed) |
| `Refund` | `original_event_ref` (optional link to the expense being refunded, if identifiable) |
| `Cashback` | `originating_expense_ref` (optional), `program_name` |
| `SubscriptionRenewal` | `subscription_name`, `renewal_cycle` (monthly/annual), `next_expected_date` |
| `FailedPayment` | `failure_reason_hint`, `related_merchant`, `attempted_amount` |
| `Salary` | `employer`, `pay_period` |
| `Investment` | `instrument_name`, `investment_action` (`buy`/`sell`/`contribution`) |
| `Dividend` | `instrument_name`, `payer` |
| `Loan` | `loan_action` (`disbursement`/`repayment`), `lender` |
| `CreditCardBill` | `statement_period`, `minimum_due`, `total_due`, `due_date` |
| `Tax` | `tax_type_hint`, `tax_authority` |
| `Donation` | `recipient_organization` |
| `Others` | Free-form `raw_hint` field for future reclassification |

> **Design Note:** New `event_type` values and their payload attributes can be added without altering the common envelope or any consumer that only relies on the envelope (amount, category, occurred_at, status). This is the mechanism by which the taxonomy stays extensible per the Vision document's non-negotiable requirement.

### 5.2 FinancialEvent Type Taxonomy

```
FinancialEvent (abstract root)
│
├── Income
│   ├── Salary
│   └── Dividend
├── Expense
├── Transfer
├── Refund
├── Cashback
├── SubscriptionRenewal
├── FailedPayment
├── Investment
├── Loan
├── CreditCardBill
├── Tax
├── Donation
└── Others
```

> **Note on hierarchy:** `Salary` and `Dividend` are modeled as specializations under the conceptual `Income` grouping for taxonomy/reporting purposes, but at the persistence and processing level (Section 5.3) all fourteen leaf types are peers sharing the same envelope — the "Income" grouping is a **reporting-level categorization**, not a structural inheritance requirement. This avoids a deep class hierarchy while preserving a meaningful grouping for Analytics (`04` → `02_product_requirement_document.md`'s Cash Flow view groups by this classification).

### 5.3 Design Decision: Single Aggregate with Polymorphic Payload vs. Table-Per-Type

**Decision:** `FinancialEvent` is one aggregate with a polymorphic/extensible payload (Section 5.1.2), **not** fourteen separate aggregate types with independent identities.

**Rationale:**
- All fourteen types share the same lifecycle (extraction → confidence gating → finalization → categorization → analytics/budget/notification consumption). Modeling them as independent aggregates would force every downstream consumer (Analytics, Budget, Notification) to special-case each type or maintain fourteen parallel query paths — a direct violation of the Vision's "Ledger is a single source of truth" principle.
- Cross-type queries (e.g., "show me all events this month regardless of type" for Cash Flow) are the **majority** of read access patterns (Analytics), while type-specific fields are accessed only in detail views or type-specific rules (Budget alert logic caring only about `Expense`). A single aggregate with an optional-attribute payload optimizes for the majority pattern without sacrificing the minority pattern.
- Extensibility (adding a 15th type) becomes an additive schema/payload change, not a new aggregate root with its own repository, service, and cross-cutting wiring.

This decision directly informs `08_database_design.md`, where it manifests as a single `financial_events` table with a typed `event_type` column and either (a) a structured JSONB payload column for type-specific attributes, or (b) narrow nullable columns — the trade-off between these two physical strategies is discussed in that document.

### 5.4 Value Object: `Provenance`

Every `FinancialEvent` must be traceable to its origin — this is a hard requirement from the Vision ("Trust Through Auditability") and the SRS (SRS-FR-AI-04, SRS-FR-LEDGER-01).

| Field | Description |
|---|---|
| `connector_type` | e.g., `gmail` |
| `connector_account_id` | Which linked account produced this |
| `source_message_ref` | Opaque reference to the raw source message (e.g., Gmail message ID) |
| `extraction_method` | `template` \| `ai` \| `manual` |
| `template_version` | Populated when `extraction_method = template` |
| `prompt_version` / `model_identifier` | Populated when `extraction_method = ai` |
| `extracted_at` | Timestamp of extraction |

`Provenance` is immutable once set — it describes history, not current state. Corrections are captured via `EventOverride`, not by mutating `Provenance`.

### 5.5 Entity: `EventOverride`

Represents a single manual correction applied by a user to a finalized `FinancialEvent`.

| Field | Description |
|---|---|
| `id` | Identifier |
| `financial_event_id` | Parent reference |
| `field_name` | Which field was corrected (`amount`, `merchant`, `event_type`, `category`, etc.) |
| `previous_value` | Value before correction (snapshot) |
| `new_value` | Value after correction |
| `actor` | User who made the correction |
| `corrected_at` | Timestamp |

**Invariant:** An `EventOverride` never deletes or overwrites the original `Provenance`-attributed value; the "effective" value of a field is computed as: *latest override for that field, if any, else the original extracted value.* This satisfies SRS-FR-LEDGER-02's requirement that automated re-processing never silently discards a user correction.

### 5.6 Entity: `CategoryAssignment`

| Field | Description |
|---|---|
| `category` | The assigned category (from a user-customizable but system-seeded taxonomy) |
| `confidence` | 0.0–1.0 |
| `source` | `user_history` \| `global_mapping` \| `ai_inference` \| `manual` |
| `assigned_at` | Timestamp |
| `superseded_by` | Optional link if a later assignment replaced this one (kept for audit, mirroring the `EventOverride` pattern) |

### 5.7 Invariants of the `FinancialEvent` Aggregate

1. **INV-1:** A `FinancialEvent` cannot transition from `pending_review` to `finalized` without a `confidence` value present and evaluated against the current threshold (enforced by the Understanding Context boundary, not bypassable via direct persistence — see `06_architecture.md` for enforcement mechanism).
2. **INV-2:** `amount.currency` is immutable once set from extraction; only `EventOverride` may adjust `amount.value`, and only within the same `currency` (a currency change requires creating a `disputed`-flagged review case, not a silent override — cross-currency correction is a Should-Have flagged for `24_future_roadmap.md`).
3. **INV-3:** `provenance.source_message_ref` combined with `provenance.connector_account_id` must be unique across all `FinancialEvent` records for a user (this is the formalization of duplicate detection, SRS-FR-PARSE-02, at the domain level — enforcement mechanism is a database constraint per `08_database_design.md`).
4. **INV-4:** A `FinancialEvent` in `archived` status (soft-deleted, e.g., due to connector disconnect + purge choice) must be excluded from all Analytics and Budget computation, but retained for the configured audit-retention window before permanent erasure.
5. **INV-5:** `event_type` is immutable post-finalization **except** via an explicit `EventOverride` on the `event_type` field itself (e.g., an event initially classified as `Expense` is later corrected to `Refund`) — this is allowed because merchant-format ambiguity between these types is a known, expected correction case, not a data-integrity violation.

### 5.8 Domain Events Emitted by Financial Core

| Domain Event | Emitted When | Consumed By |
|---|---|---|
| `FinancialEventCandidateCreated` | A `CandidateFinancialEvent` is produced by either extraction path | Confidence Gating policy |
| `FinancialEventPendingReview` | Confidence below threshold | Notification (review-request), Administration |
| `FinancialEventFinalized` | Confidence at/above threshold, or user confirms a pending-review item | Analytics, Budget Engine, Notification |
| `FinancialEventOverridden` | A user submits a manual correction | Analytics (recompute-on-read, no separate handling needed given CQRS read models), Administration (audit) |
| `FinancialEventArchived` | Soft-delete due to purge/disconnect | Analytics, Budget Engine (exclude going forward) |
| `CategoryAssignmentChanged` | A new categorization (automated or manual) is applied | Analytics, AI Engine (feedback loop into `user_history` source) |

---

## 6. The Ingestion Context (Supporting Domain)

### 6.1 Aggregate Root: `GmailAccount`

Represents a linked Gmail mailbox. Owns synchronization state as a cohesive aggregate, since `historyId`, watch expiry, and sync status must be updated atomically to avoid drift.

| Field | Description |
|---|---|
| `id` | Identifier |
| `user_id` | Owning user |
| `email_address` | The linked mailbox address |
| `oauth_token_ref` | Reference to encrypted token storage (Identity context owns actual token material — see Section 8) |
| `sync_state.last_history_id` | Last successfully processed Gmail `historyId` |
| `sync_state.watch_expiration` | When the current Gmail Watch expires |
| `sync_state.status` | `initial_sync` \| `active` \| `disconnected` \| `error` |

### 6.2 Entity: `IngestedMessage`

Represents a raw signal that has entered the platform but not yet been interpreted.

| Field | Description |
|---|---|
| `id` | Identifier |
| `gmail_account_id` | Parent reference |
| `gmail_message_id` | External identifier (used for INV-3 dedup) |
| `raw_content_ref` | Pointer to time-boxed raw content storage (Section 6.3 on data lifecycle) |
| `received_at` | When Gmail received the message |
| `routing_decision` | `template_parser` \| `ai_extractor` \| `discarded` |
| `processing_status` | `queued` \| `processing` \| `completed` \| `failed` |

### 6.3 Data Lifecycle Note

Per SRS Section 6.7, `raw_content_ref` points to storage governed by a time-boxed retention policy. The domain model treats raw content as **transient supporting data**, not part of the permanent `FinancialEvent` aggregate — this is a deliberate boundary so that raw-content purging never touches the `FinancialEvent` aggregate's integrity or its `Provenance` (which retains only an opaque reference, not the content itself, post-purge).

### 6.4 Domain Events

| Domain Event | Emitted When | Consumed By |
|---|---|---|
| `MessageIngested` | New raw message accepted | Understanding Context (routing/extraction trigger) |
| `WatchRenewed` / `WatchExpiringSoon` | Gmail Watch lifecycle | Administration (Connector Monitoring) |
| `HistorySyncFallbackTriggered` | `historyId` invalid, bounded re-sync used | Administration (Connector Monitoring) |
| `AccountDisconnected` | User disconnects or token revoked | Financial Core (archive events, if purge chosen), Notification (reconnection prompt) |

---

## 7. The Understanding Context (Supporting Domain)

### 7.1 Aggregate Root: `SenderTemplate`

A versioned, deterministic extraction rule set for a known sender.

| Field | Description |
|---|---|
| `id` | Identifier |
| `sender_pattern` | Domain/address matching rule |
| `version` | Monotonic version number |
| `extraction_rules` | Selector/regex definitions (opaque to the domain model — interpreted by the Parser engine, detailed in `13_email_parser.md`) |
| `failure_rate` | Rolling metric (read-model, updated by Administration context, not owned transactionally here) |
| `active` | Whether this version is currently applied |

### 7.2 Value Object: `CandidateFinancialEvent`

The shared output shape of both extraction paths (Template and AI) — this is the object that eventually becomes a `FinancialEvent` upon passing confidence gating. Its shape **exactly mirrors** the `FinancialEvent` common envelope plus type-specific payload (Section 5.1), with the addition of a not-yet-persisted status. This symmetry is intentional: it means confidence gating and finalization are a pure mapping/promotion operation, not a data-reshaping operation.

### 7.3 Domain Events

| Domain Event | Emitted When | Consumed By |
|---|---|---|
| `TemplateExtractionSucceeded` / `TemplateExtractionFailed` | Per template parse attempt | AI Engine (fallback trigger on failure), Administration (Parser Monitoring) |
| `AIExtractionCompleted` | AI extractor produces a candidate | Financial Core (confidence gating) |
| `AIExtractionSchemaInvalid` | LLM response fails schema validation after re-prompt | Manual review queue, Administration (Prompt Monitoring) |

---

## 8. Identity & Access Context (Generic Subdomain)

### 8.1 Aggregate Root: `User`

| Field | Description |
|---|---|
| `id` | Identifier |
| `email` | Primary identity (from Google profile) |
| `settings` | Notification preferences, thresholds, timezone |
| `linked_gmail_accounts` | Reference collection (owned by Ingestion Context; `User` holds references, not the accounts themselves — avoiding cross-context aggregate entanglement) |

### 8.2 Entity: `OAuthToken`

Deliberately modeled as owned by the Identity & Access context (not the Ingestion context), since token security/lifecycle policy (encryption, refresh, revocation detection) is a generic-subdomain concern distinct from Gmail-specific sync mechanics.

| Field | Description |
|---|---|
| `id` | Identifier |
| `gmail_account_id` | Reference (cross-context reference by ID only) |
| `encrypted_access_token` | Envelope-encrypted |
| `encrypted_refresh_token` | Envelope-encrypted |
| `expires_at` | Access token expiry |

---

## 9. Budget Context (Supporting Domain)

### 9.1 Aggregate Root: `Budget`

| Field | Description |
|---|---|
| `id` | Identifier |
| `user_id` | Owning user |
| `scope` | `global` \| `category:<category_name>` |
| `period_type` | `monthly` (extensible) |
| `threshold_amount` | Money value object |
| `alert_thresholds` | e.g., `[0.8, 1.0]` — configurable percentage crossings |
| `fired_alerts` | Set of `(period_start, threshold)` pairs already alerted — enforces idempotent alerting (SRS-FR-BUDGET-01/02) |

### 9.2 Entity: `SavingsGoal`

| Field | Description |
|---|---|
| `id` | Identifier |
| `user_id` | Owning user |
| `target_amount` | Money |
| `target_date` | Date |
| `progress_strategy` | How progress is computed from net cash flow (detailed in `02_product_requirement_document.md` Phase 2/3 scope) |

### 9.3 Domain Events

| Domain Event | Emitted When | Consumed By |
|---|---|---|
| `BudgetThresholdCrossed` | Cumulative spend crosses a configured, not-yet-fired threshold | Notification |
| `SavingsGoalProgressUpdated` | Recomputation triggered by new finalized events | Notification (periodic), Analytics |

---

## 10. Analytics Context (Read-Model / CQRS Boundary)

Analytics is explicitly modeled as a **read-model context with no aggregates of its own** in the write-model sense. Its "entities" (`CashFlowView`, `CategoryBreakdown`, `MerchantRanking`, `TrendView`) are derived, rebuildable projections over `FinancialEventFinalized` / `FinancialEventOverridden` / `FinancialEventArchived` domain events from the Financial Core.

**Design Rule:** Analytics never issues commands back to Financial Core. It is a pure consumer. This one-way dependency is what makes the CQRS split safe and keeps the Financial Core context uncoupled from reporting concerns — fully elaborated in `06_architecture.md`.

---

## 11. Engagement Context (Supporting Domain)

### 11.1 Aggregate Root: `NotificationJob`

| Field | Description |
|---|---|
| `id` | Identifier |
| `user_id` | Recipient |
| `notification_type` | `daily_summary` \| `weekly_summary` \| `monthly_report` \| `budget_alert` \| `ai_recommendation` \| `review_request` |
| `payload_ref` | Reference to composed content (built from Analytics read models) |
| `delivery_status` | `pending` \| `delivered` \| `failed` \| `retrying` |

### 11.2 Domain Events

| Domain Event | Emitted When | Consumed By |
|---|---|---|
| `NotificationComposed` | Content ready for delivery | WhatsApp delivery adapter |
| `NotificationDelivered` / `NotificationFailed` | Delivery attempt outcome | Administration |

---

## 12. Administration Context (Cross-Cutting Supporting Domain)

Modeled as a set of **read-models and an append-only log**, not transactional aggregates that other contexts depend on for correctness:

- `ConnectorHealthRecord` — derived from Ingestion Context events.
- `ParserMetric` — derived from Understanding Context events.
- `PromptMetric` — derived from Understanding Context AI events.
- `AuditLog` — append-only, derived from **every** context's state-changing domain events (a cross-cutting subscriber, not a bounded context with its own business rules).

**Design Rule:** No context may query Administration's read-models to make a business decision (e.g., Budget Engine must not depend on `ParserMetric`). Administration is strictly downstream — a one-way observability plane.

---

## 13. Cross-Context Interaction Rules

1. Contexts communicate exclusively via **domain events** (in-process for MVP's modular monolith, transportable to a message broker without domain model change later — per the Vision's microservice-readiness principle).
2. No context directly queries another context's write-model tables. Cross-context reads happen only via published read-models (Analytics) or explicit reference IDs resolved through the owning context's own API/service (never a raw join across schemas) — enforced at the architecture level in `06_architecture.md` and at the schema level in `08_database_design.md`.
3. Identifiers are the only thing shared by value across context boundaries (e.g., Financial Core stores `connector_account_id` as an opaque reference, not a full `GmailAccount` object).

---

## 14. Detailed Explanation: Confidence as a First-Class Value Object

`Confidence` is modeled as a value object (not a bare float) because it carries structure that the domain relies on:

- An **aggregate confidence** (used for the gating invariant, INV-1).
- Optionally, **per-field confidence** (amount, merchant, date, category individually) — used by the review-queue UI to highlight *which* field a user should check, rather than presenting the whole event as suspect.
- A **computation method tag** (`template_deterministic` vs. `ai_weighted`) — because a template's confidence of 1.0 has different epistemic meaning than an AI's confidence of 1.0, and Administration/Prompt Monitoring needs to distinguish these when analyzing accuracy trends.

This richness is why `Confidence` is a value object attached to both `CandidateFinancialEvent` and the finalized `FinancialEvent`, rather than a transient scoring output discarded after gating.

---

## 15. Component Responsibilities (Domain-Level)

| Context | Responsibility |
|---|---|
| Identity & Access | Own user identity and token lifecycle; expose nothing about email content |
| Ingestion | Own mailbox sync state and raw message capture; own no interpretation logic |
| Understanding | Own extraction (deterministic + AI) and produce symmetric `CandidateFinancialEvent` outputs |
| Financial Core | Own the `FinancialEvent` aggregate, its invariants, overrides, and categorization — the system of record |
| Budget | Own threshold definitions and idempotent alert-firing logic |
| Analytics | Own derived, rebuildable read-models; no write authority over the ledger |
| Engagement | Own notification composition and delivery tracking |
| Administration | Own cross-cutting observability and audit, strictly downstream of all other contexts |

---

## 16. Trade-offs

| Decision | Trade-off |
|---|---|
| Single polymorphic `FinancialEvent` aggregate vs. per-type aggregates | Slightly more complex payload modeling, in exchange for a unified ledger and vastly simpler cross-type consumers (Analytics, Budget). |
| Append-only + override pattern instead of destructive updates | Higher storage footprint and query complexity (effective-value resolution), in exchange for full auditability — non-negotiable for a financial system. |
| Cross-context communication via domain events only | Slightly higher short-term implementation ceremony (event definitions, handlers) vs. direct method calls, in exchange for microservice-extraction readiness. |
| Raw content excluded from the `FinancialEvent` aggregate itself | Requires a secondary reference-resolution step for debugging, in exchange for a clean separation between permanent financial record and transient supporting data. |

## 17. Advantages
- The domain model directly operationalizes the Vision's `FinancialEvent`-as-root mandate in concrete, buildable DDD terms.
- Confidence gating and override handling are modeled as first-class domain concepts (not application-layer afterthoughts), ensuring they can't be accidentally bypassed by a future feature.
- Bounded context boundaries map cleanly onto the Vision's modular-monolith-to-microservices trajectory.

## 18. Disadvantages
- The polymorphic payload approach requires careful schema design (`08_database_design.md`) to avoid becoming an unstructured "junk drawer" — this is a known risk that document must explicitly address (structured JSONB with a documented per-type schema, not free-form).
- Fourteen event types with individually meaningful attributes represent nontrivial ongoing domain maintenance as the taxonomy grows (Section 5.1.2's `Others` catch-all mitigates but does not eliminate this).

## 19. Future Improvements
- Introduce explicit `linked_transfer_event_id` resolution logic once both legs of a transfer are reliably observable (currently optional/best-effort).
- Model `Investment`/`Dividend` more richly (holdings, cost basis) if/when a net-worth tracking feature is prioritized (`24_future_roadmap.md`).
- Consider promoting `Budget` and `SavingsGoal` into a richer "Financial Planning" context if Phase 3 advisory features (predictive cash flow) require more sophisticated planning-domain concepts than currently modeled.

## 20. Best Practices
- Any new `event_type` must be added to Section 5.2's taxonomy and Section 5.1.2's payload table in this document **before** implementation, to keep this document authoritative.
- Cross-context references must always be by opaque ID, never by embedding another context's entity/value object directly.
- Every new domain event must be added to its context's event table in this document and cross-checked against `05_event_storming.md` for consistency.

## 21. Production Considerations
- The override/audit trail (`EventOverride`, superseded `CategoryAssignment`) will grow unboundedly over a user's lifetime; retention/archival strategy for override history (distinct from the raw content retention policy in Section 6.3) must be addressed in `08_database_design.md`.
- Confidence value object's per-field breakdown must be designed with the review-queue UX in mind from day one — retrofitting per-field confidence after launch would require reprocessing historical AI extractions or accepting a data gap for pre-launch records.
- INV-3 (dedup uniqueness) must be enforced at the database constraint level, not merely in application code, to survive concurrent processing paths (multiple workers, retry races) — this is carried forward explicitly into `08_database_design.md`.

---

**Next Document:** `05_event_storming.md` — captures the full event-storming workshop output (commands, events, policies, external systems, hotspots) that operationalizes this domain model into a time-ordered process view, bridging toward the technical architecture in `06_architecture.md`.
