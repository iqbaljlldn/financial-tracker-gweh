# Database Design
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `05_event_storming.md`, `06_architecture.md`, `07_sequence_diagrams.md`
**Feeds Into:** `09_api_specification.md`, `10_backend_blueprint.md`, `21_project_structure.md`

**Naming Convention Note:** Per project convention, all database identifiers (schemas, tables, columns, indexes, constraints) use **snake_case**. Application-layer (Go) identifiers use **camelCase**, mapped via sqlc's generated structs — the mapping convention itself is detailed in `10_backend_blueprint.md` and `20_coding_convention.md`.

---

## 1. Purpose

This document translates the domain model (`04_domain_model.md`), the module-schema-ownership model (`06_architecture.md`), and the transaction boundaries demonstrated in `07_sequence_diagrams.md` into a concrete PostgreSQL physical schema: tables, columns, types, indexes, constraints, and the Outbox/Inbox tables that make the event-driven architecture correct.

Every table in this document is owned by exactly one module's PostgreSQL schema, per the architecture's non-negotiable module-boundary rule. No table here has a foreign key referencing a table in another module's schema — cross-module references are stored as plain UUID columns, validated at the application layer, exactly as specified in `06_architecture.md` Section 5.2.

## 2. Scope

### 2.1 In Scope
- Full DDL-level schema design (described in structured tables here; actual Goose migration files are an implementation deliverable, not reproduced verbatim in this document per the "no implementation code" directive — this document specifies precisely enough that writing the migrations is a mechanical exercise).
- Per-module schema: `identity`, `ingestion`, `understanding`, `financialcore`, `budget`, `analytics`, `engagement`, `administration`.
- Outbox and Inbox table design (shared pattern, instantiated per module).
- Indexing strategy, partitioning considerations, and constraint design.
- The `financial_events` polymorphic payload strategy (resolving the design question flagged in `04_domain_model.md` Section 18).

### 2.2 Out of Scope
- Actual Goose migration file syntax/ordering → implementation phase.
- Query-level SQL (sqlc query files) → `10_backend_blueprint.md`.
- API-level DTOs → `09_api_specification.md`.

---

## 3. Architecture: Schema-per-Module Layout

```
PostgreSQL Cluster
│
├── schema: identity
│   ├── users
│   └── oauth_tokens
│
├── schema: ingestion
│   ├── gmail_accounts
│   ├── ingested_messages
│   ├── sender_watch_log
│   └── outbox_events (ingestion)
│
├── schema: understanding
│   ├── sender_templates
│   ├── extraction_results
│   └── outbox_events (understanding)
│
├── schema: financialcore
│   ├── financial_events
│   ├── event_overrides
│   ├── category_assignments
│   ├── inbox_processed (financialcore)
│   └── outbox_events (financialcore)
│
├── schema: budget
│   ├── budgets
│   ├── budget_fired_alerts
│   ├── savings_goals
│   └── outbox_events (budget)
│
├── schema: analytics
│   ├── cash_flow_view
│   ├── category_breakdown_view
│   ├── merchant_ranking_view
│   ├── trend_view
│   └── projection_checkpoint
│
├── schema: engagement
│   ├── notification_jobs
│   ├── notification_preferences
│   ├── inbox_processed (engagement)
│   └── outbox_events (engagement)
│
└── schema: administration
    ├── audit_log
    ├── connector_health_metrics
    ├── parser_metrics
    └── prompt_metrics
```

Each module's database role (`identity_svc`, `ingestion_svc`, etc.) is granted `USAGE` + `SELECT/INSERT/UPDATE/DELETE` **only** on its own schema, per `06_architecture.md` Section 5.3's defense-in-depth requirement.

---

## 4. Shared Pattern: Outbox Table

Every module that emits domain events owns an identically-shaped `outbox_events` table within its own schema (namespacing avoids a single hot table shared across modules, which would itself become a boundary violation and a contention point).

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key, generated (`gen_random_uuid()`) |
| `aggregate_type` | `text` | e.g., `financial_event`, `gmail_account` |
| `aggregate_id` | `uuid` | The aggregate instance this event pertains to |
| `event_type` | `text` | e.g., `FinancialEventFinalized` |
| `payload` | `jsonb` | Event payload, schema-versioned (see `payload_schema_version`) |
| `payload_schema_version` | `smallint` | Enables safe payload evolution over time |
| `created_at` | `timestamptz` | Default `now()` |
| `relayed` | `boolean` | Default `false` |
| `relayed_at` | `timestamptz` | Nullable |

**Indexes:**
- `idx_outbox_events_unrelayed` — partial index `WHERE relayed = false`, ordered by `created_at`, used by the Outbox Relay's `SELECT ... FOR UPDATE SKIP LOCKED` poll query (per `06_architecture.md` Section 6.1) — a partial index keeps this hot-path index small regardless of historical outbox volume.

**Retention:** Relayed rows are retained for a configurable window (default 30 days) before archival/deletion via a scheduled cleanup job — long enough to support the Analytics read-model rebuildability requirement (SRS Section 6.3) via replay if needed, without retaining the outbox table indefinitely as a full event store (a dedicated event-log/replay store is a noted future improvement, Section 18).

---

## 5. Shared Pattern: Inbox Table

Modules whose task handlers lack a naturally idempotent side effect (per `06_architecture.md` Section 6.2) own an `inbox_processed` table.

| Column | Type | Notes |
|---|---|---|
| `task_id` | `text` | Primary key — the Asynq task's unique identifier or a domain-derived idempotency key |
| `processed_at` | `timestamptz` | Default `now()` |
| `result_summary` | `jsonb` | Optional — small metadata about the outcome, for debugging |

Where a natural idempotency key already exists as a unique constraint on the target table itself (e.g., `financial_events`'s `(connector_account_id, source_message_ref)` constraint — Section 7.1), that constraint **is** the Inbox mechanism, and a separate `inbox_processed` table is not duplicated for that path. `inbox_processed` is used specifically for modules like `engagement` (WhatsApp delivery), where the side effect (an external API call) has no natural database-level uniqueness to lean on.

---

## 6. Schema: `identity`

### 6.1 Table: `users`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `email` | `citext` | `UNIQUE NOT NULL` |
| `display_name` | `text` | Nullable |
| `timezone` | `text` | `NOT NULL DEFAULT 'UTC'` |
| `notification_daily_time` | `time` | Nullable, user-configurable |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` |
| `updated_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

### 6.2 Table: `oauth_tokens`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `gmail_account_id` | `uuid` | `NOT NULL` — cross-schema reference by ID only (no FK, per boundary rule) |
| `encrypted_access_token` | `bytea` | `NOT NULL` — envelope-encrypted (KMS-wrapped DEK; see `16_security.md`) |
| `encrypted_refresh_token` | `bytea` | `NOT NULL` |
| `access_token_expires_at` | `timestamptz` | `NOT NULL` |
| `scope` | `text` | `NOT NULL` — recorded for audit of exactly which scopes were granted |
| `created_at` / `updated_at` | `timestamptz` | Standard |

**Index:** `idx_oauth_tokens_gmail_account_id` on `gmail_account_id` (frequent lookup during token refresh).

---

## 7. Schema: `financialcore` (Core Domain — Highest Design Scrutiny)

### 7.1 Table: `financial_events`

This is the physical realization of the `FinancialEvent` aggregate root from `04_domain_model.md`. It resolves the polymorphic-payload design question via a **hybrid strategy**: narrow, strongly-typed columns for fields shared and queried across *all* event types (used heavily by Analytics/Budget), plus a structured `jsonb` column for type-specific attributes (Section 5.1.2 of the domain model), validated at the application layer against a **per-`event_type` documented JSON schema** (not a free-form blob — the schemas themselves are maintained as versioned artifacts referenced from `04_domain_model.md` Section 5.1.2, one schema per event type).

| Column | Type | Constraints / Notes |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | `NOT NULL` — indexed, cross-schema reference |
| `event_type` | `text` | `NOT NULL` — CHECK constraint against the enumerated taxonomy (Income, Expense, Transfer, Refund, Cashback, SubscriptionRenewal, FailedPayment, Salary, Investment, Dividend, Loan, CreditCardBill, Tax, Donation, Others) |
| `amount_value` | `numeric(18,4)` | `NOT NULL` — fixed-point, never `float`, for monetary correctness |
| `amount_currency` | `char(3)` | `NOT NULL` — ISO 4217 code |
| `occurred_at` | `timestamptz` | `NOT NULL` — indexed, primary time dimension for analytics |
| `merchant_raw_name` | `text` | Nullable |
| `merchant_normalized_name` | `text` | Nullable — indexed, used for Merchant Ranking |
| `payment_method_type` | `text` | Nullable (`card`, `wallet`, `bank_transfer`, etc.) |
| `payment_method_hint` | `text` | Nullable (e.g., masked card last-4) |
| `status` | `text` | `NOT NULL DEFAULT 'pending_review'` — CHECK IN (`pending_review`, `finalized`, `archived`, `disputed`) |
| `confidence_aggregate` | `numeric(4,3)` | `NOT NULL` — 0.000–1.000 |
| `confidence_method` | `text` | `NOT NULL` — CHECK IN (`template_deterministic`, `ai_weighted`, `manual`) |
| `type_specific_payload` | `jsonb` | `NOT NULL DEFAULT '{}'` — validated per-`event_type` schema at application layer |
| `provenance_connector_type` | `text` | `NOT NULL` (e.g., `gmail`) |
| `provenance_connector_account_id` | `uuid` | `NOT NULL` |
| `provenance_source_message_ref` | `text` | `NOT NULL` |
| `provenance_extraction_method` | `text` | `NOT NULL` — CHECK IN (`template`, `ai`, `manual`) |
| `provenance_template_version` | `text` | Nullable |
| `provenance_prompt_version` | `text` | Nullable |
| `provenance_model_identifier` | `text` | Nullable |
| `provenance_extracted_at` | `timestamptz` | `NOT NULL` |
| `created_at` / `updated_at` | `timestamptz` | Standard |

**Constraints:**
- `UNIQUE (provenance_connector_account_id, provenance_source_message_ref)` — **this is the physical enforcement of INV-3** (no duplicate `FinancialEvent` per source message), and the exact mechanism demonstrated in `07_sequence_diagrams.md` Diagram 5's `ON CONFLICT DO NOTHING` idempotency.
- `CHECK (confidence_aggregate BETWEEN 0 AND 1)`
- `CHECK (amount_currency ~ '^[A-Z]{3}$')`

**Indexes:**
- `idx_financial_events_user_occurred` on `(user_id, occurred_at DESC)` — primary access pattern for both the ledger view and as the seed for Analytics projection.
- `idx_financial_events_user_status` on `(user_id, status)` — partial-friendly for filtering `pending_review` items (review-queue UI) or excluding `archived` (Analytics/Budget queries per INV-4).
- `idx_financial_events_merchant` on `(user_id, merchant_normalized_name)` — Merchant Ranking support.
- GIN index on `type_specific_payload` (`idx_financial_events_payload_gin`) — supports occasional type-specific filtering (e.g., "show me all `CreditCardBill` events with `due_date` in the next 7 days") without requiring a schema migration per new query need.

**Partitioning Consideration:** At the scale targets in `03_software_requirement_specification.md` Section 6.5 (100,000+ mailboxes), `financial_events` is the platform's highest-volume table. It is designed from day one to be **range-partitioned by `occurred_at`** (e.g., monthly partitions), even if partitioning is not activated until volume warrants it — the schema avoids any design choice (like a non-time-leading primary key) that would make retrofitting partitioning difficult later. This is elaborated in `23_scalability.md`.

### 7.2 Table: `event_overrides`

Physical realization of the `EventOverride` entity (`04_domain_model.md` Section 5.5).

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `financial_event_id` | `uuid` | `NOT NULL`, references `financial_events(id)` — **this FK is intra-schema and IS allowed**, since both tables live in `financialcore` |
| `field_name` | `text` | `NOT NULL` |
| `previous_value` | `jsonb` | `NOT NULL` — snapshot, type-flexible across different field types |
| `new_value` | `jsonb` | `NOT NULL` |
| `actor_user_id` | `uuid` | `NOT NULL` |
| `corrected_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Index:** `idx_event_overrides_financial_event_id` on `financial_event_id` (append-only audit trail lookup per event, and used to compute "effective value" per `04_domain_model.md`'s override-resolution rule).

### 7.3 Table: `category_assignments`

Physical realization of the `CategoryAssignment` entity (`04_domain_model.md` Section 5.6), modeled as its own append-only-with-supersession table (not a mutable column on `financial_events`) specifically to preserve the categorization audit trail required for Prompt Monitoring analysis.

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `financial_event_id` | `uuid` | `NOT NULL`, FK to `financial_events(id)` |
| `category` | `text` | `NOT NULL` |
| `confidence` | `numeric(4,3)` | `NOT NULL` |
| `source` | `text` | `NOT NULL` — CHECK IN (`user_history`, `global_mapping`, `ai_inference`, `manual`) |
| `assigned_at` | `timestamptz` | `NOT NULL DEFAULT now()` |
| `superseded_by` | `uuid` | Nullable, self-referencing FK to `category_assignments(id)` |
| `is_current` | `boolean` | `NOT NULL DEFAULT true` — denormalized flag for fast "current category" lookups, maintained transactionally alongside `superseded_by` |

**Index:** `idx_category_assignments_current` — partial index `WHERE is_current = true`, used by Analytics's Category Breakdown projection.

### 7.4 Table: `inbox_processed` (financialcore)

Per Section 5 — used sparingly here since `financial_events`'s own unique constraint covers the primary dedup need; this table exists for any financialcore-owned task without a natural constraint to lean on (e.g., `ConfirmPendingReviewEvent` command processing, guarding against a double-tap "Y" reply race).

*(Shape as defined in Section 5.)*

---

## 8. Schema: `ingestion`

### 8.1 Table: `gmail_accounts`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | `NOT NULL` — indexed |
| `email_address` | `citext` | `UNIQUE NOT NULL` — enforces H1's resolution (no duplicate linking across the platform) |
| `status` | `text` | `NOT NULL` — CHECK IN (`linking`, `initial_sync`, `active`, `disconnected`, `error`) |
| `last_history_id` | `text` | Nullable — Gmail's opaque history cursor |
| `watch_expiration` | `timestamptz` | Nullable |
| `initial_sync_window_days` | `smallint` | `NOT NULL DEFAULT 90` |
| `created_at` / `updated_at` | `timestamptz` | Standard |

### 8.2 Table: `ingested_messages`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `gmail_account_id` | `uuid` | `NOT NULL`, FK to `gmail_accounts(id)` (intra-schema, allowed) |
| `gmail_message_id` | `text` | `NOT NULL` |
| `received_at` | `timestamptz` | `NOT NULL` |
| `raw_content_ref` | `text` | Nullable — pointer to time-boxed object storage (MinIO), per `04_domain_model.md` Section 6.3's data lifecycle note |
| `routing_decision` | `text` | Nullable — CHECK IN (`template_parser`, `ai_extractor`, `discarded`) |
| `processing_status` | `text` | `NOT NULL DEFAULT 'queued'` — CHECK IN (`queued`, `processing`, `completed`, `failed`) |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Constraint:** `UNIQUE (gmail_account_id, gmail_message_id)` — the ingestion-level duplicate guard (distinct from, and earlier in the pipeline than, INV-3 on `financial_events`).

**Index:** `idx_ingested_messages_status` — partial `WHERE processing_status IN ('queued','failed')`, used for stuck-message monitoring.

### 8.3 Table: `sender_watch_log`

Append-only log of watch registration/renewal events, supporting Connector Monitoring (FR-ADMIN-01) without requiring Administration to reach into `ingestion`'s primary tables.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK |
| `gmail_account_id` | `uuid` | `NOT NULL` |
| `event` | `text` | `registered` \| `renewed` \| `renewal_failed` \| `expired` |
| `occurred_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

---

## 9. Schema: `understanding`

### 9.1 Table: `sender_templates`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `sender_pattern` | `text` | `NOT NULL` — domain/address matcher |
| `version` | `integer` | `NOT NULL` |
| `extraction_rules` | `jsonb` | `NOT NULL` — selector/regex definitions, opaque to this document, detailed in `13_email_parser.md` |
| `active` | `boolean` | `NOT NULL DEFAULT true` |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Constraint:** `UNIQUE (sender_pattern, version)`.
**Index:** `idx_sender_templates_active` — partial `WHERE active = true`, the hot lookup path for every ingested message's routing decision (also cached in Redis per `06_architecture.md` Section 13's caching note).

### 9.2 Table: `extraction_results`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `ingested_message_id` | `uuid` | `NOT NULL` — cross-schema reference by ID (to `ingestion.ingested_messages`) |
| `method` | `text` | `NOT NULL` — `template` \| `ai` |
| `template_version` | `text` | Nullable |
| `prompt_version` | `text` | Nullable |
| `model_identifier` | `text` | Nullable |
| `candidate_payload` | `jsonb` | `NOT NULL` — the full `CandidateFinancialEvent` shape, mirroring `financial_events`'s structure per `04_domain_model.md` Section 7.2's symmetry design |
| `confidence_aggregate` | `numeric(4,3)` | `NOT NULL` |
| `confidence_per_field` | `jsonb` | Nullable — supports the review-queue UI's field-level confidence highlighting |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Index:** `idx_extraction_results_ingested_message_id`.

---

## 10. Schema: `budget`

### 10.1 Table: `budgets`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | `NOT NULL` — indexed |
| `scope` | `text` | `NOT NULL` — `global` or `category:<name>` |
| `period_type` | `text` | `NOT NULL DEFAULT 'monthly'` |
| `threshold_amount_value` | `numeric(18,4)` | `NOT NULL` |
| `threshold_amount_currency` | `char(3)` | `NOT NULL` |
| `alert_thresholds` | `numeric(3,2)[]` | `NOT NULL DEFAULT '{0.8,1.0}'` |
| `active` | `boolean` | `NOT NULL DEFAULT true` |
| `created_at` / `updated_at` | `timestamptz` | Standard |

### 10.2 Table: `budget_fired_alerts`

Physical realization of `Budget.fired_alerts` (`04_domain_model.md` Section 9.1) — extracted into its own table (rather than an array column) to keep the idempotency check a simple indexed lookup rather than an array-scan.

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `budget_id` | `uuid` | `NOT NULL`, FK to `budgets(id)` |
| `period_start` | `date` | `NOT NULL` |
| `threshold` | `numeric(3,2)` | `NOT NULL` |
| `fired_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Constraint:** `UNIQUE (budget_id, period_start, threshold)` — this **is** the idempotent-alerting enforcement mechanism at the database level, matching Diagram 6's design.

### 10.3 Table: `savings_goals`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | `NOT NULL` |
| `target_amount_value` | `numeric(18,4)` | `NOT NULL` |
| `target_amount_currency` | `char(3)` | `NOT NULL` |
| `target_date` | `date` | `NOT NULL` |
| `created_at` / `updated_at` | `timestamptz` | Standard |

---

## 11. Schema: `analytics` (Read-Model / CQRS Side)

These tables are **derived, rebuildable projections** — per `06_architecture.md` Section 7, they must never be treated as a secondary source of truth requiring backup/restore parity with `financialcore` (though they are backed up operationally for availability, not correctness).

### 11.1 Table: `cash_flow_view`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `uuid` | Part of composite PK |
| `period_bucket` | `date` | Part of composite PK — truncated to day or month depending on `granularity` |
| `granularity` | `text` | Part of composite PK — `daily` \| `monthly` |
| `total_income_value` | `numeric(18,4)` | Denormalized aggregate |
| `total_expense_value` | `numeric(18,4)` | Denormalized aggregate |
| `currency` | `char(3)` | Aggregation currency (see Section 14 on multi-currency) |
| `updated_at` | `timestamptz` | Projection freshness marker, feeds `analytics_projection_lag_seconds` metric |

**Primary Key:** `(user_id, period_bucket, granularity, currency)`.

### 11.2 Table: `category_breakdown_view`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `uuid` | Composite PK |
| `period_bucket` | `date` | Composite PK |
| `category` | `text` | Composite PK |
| `total_value` | `numeric(18,4)` | |
| `currency` | `char(3)` | Composite PK |
| `percentage_share` | `numeric(5,2)` | Computed at projection time |
| `updated_at` | `timestamptz` | |

### 11.3 Table: `merchant_ranking_view`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `uuid` | Composite PK |
| `period_bucket` | `date` | Composite PK |
| `merchant_normalized_name` | `text` | Composite PK |
| `total_value` | `numeric(18,4)` | |
| `transaction_count` | `integer` | |
| `currency` | `char(3)` | Composite PK |
| `updated_at` | `timestamptz` | |

### 11.4 Table: `trend_view`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `uuid` | Composite PK |
| `period_bucket` | `date` | Composite PK |
| `granularity` | `text` | Composite PK — `daily` \| `monthly` |
| `net_flow_value` | `numeric(18,4)` | `income - expense` |
| `currency` | `char(3)` | Composite PK |
| `updated_at` | `timestamptz` | |

### 11.5 Table: `projection_checkpoint`

Tracks the last successfully-projected Outbox event per source module, enabling both crash-recovery resume and full rebuildability (SRS Section 6.3).

| Column | Type | Notes |
|---|---|---|
| `source_module` | `text` | PK component (e.g., `financialcore`) |
| `last_processed_outbox_event_id` | `uuid` | PK component's paired value |
| `checkpoint_at` | `timestamptz` | |

---

## 12. Schema: `engagement`

### 12.1 Table: `notification_jobs`

| Column | Type | Constraints |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | `NOT NULL` — indexed |
| `notification_type` | `text` | `NOT NULL` — CHECK IN (`daily_summary`, `weekly_summary`, `monthly_report`, `budget_alert`, `ai_recommendation`, `review_request`) |
| `composed_payload` | `jsonb` | `NOT NULL` |
| `delivery_status` | `text` | `NOT NULL DEFAULT 'pending'` — CHECK IN (`pending`, `delivered`, `failed`, `retrying`) |
| `provider_message_id` | `text` | Nullable — WhatsApp's own message ID, for webhook correlation |
| `delivery_confirmed_at` | `timestamptz` | Nullable — set by the async delivery webhook (Diagram 7) |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Index:** `idx_notification_jobs_provider_message_id` — supports fast webhook-driven lookup.

### 12.2 Table: `notification_preferences`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `uuid` | PK |
| `daily_summary_enabled` | `boolean` | `NOT NULL DEFAULT true` |
| `weekly_summary_enabled` | `boolean` | `NOT NULL DEFAULT true` |
| `monthly_report_enabled` | `boolean` | `NOT NULL DEFAULT true` |
| `budget_alerts_enabled` | `boolean` | `NOT NULL DEFAULT true` |
| `ai_recommendations_enabled` | `boolean` | `NOT NULL DEFAULT false` — opt-in, per Phase 3 scope |

---

## 13. Schema: `administration`

### 13.1 Table: `audit_log`

Append-only; **never** subject to the purge cascade's erasure step (per `07_sequence_diagrams.md` Diagram 8's explicit exemption).

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | Nullable (some audit entries are system-level, not user-scoped) |
| `action` | `text` | `NOT NULL` — e.g., `gmail_account_disconnected`, `data_purge_completed`, `manual_override_submitted` |
| `actor` | `text` | `NOT NULL` — `user`, `system`, or a specific service name |
| `metadata` | `jsonb` | `NOT NULL DEFAULT '{}'` |
| `occurred_at` | `timestamptz` | `NOT NULL DEFAULT now()` |

**Note:** This table has no `DELETE` grant in the `administration_svc` role's permissions at all — deletion is only possible via a superuser-executed, separately audited data-retention-law-mandated process, never via application code. This is a deliberate defense-in-depth choice for the one table that must survive every other cascade.

### 13.2 Tables: `connector_health_metrics`, `parser_metrics`, `prompt_metrics`

These are rollup/aggregate tables populated by scheduled jobs consuming the respective modules' domain events (Connector Monitoring, Parser Monitoring, Prompt Monitoring per FR-ADMIN-01/02/03). Structurally similar time-bucketed metric tables; full column-level design deferred to `17_observability.md` since these are primarily observability infrastructure rather than core domain data, and their exact shape is best co-designed with the Prometheus/Grafana metric catalog to avoid duplication.

---

## 14. Detailed Explanation: Multi-Currency Aggregation Strategy

Per `03_software_requirement_specification.md` Section 6.4, raw `financial_events.amount_value`/`amount_currency` are never lossy-converted. The Analytics schema (Section 11) handles this by:

1. **Primary strategy — currency-partitioned aggregates:** Every analytics table includes `currency` as part of its primary key. A user with both USD and IDR transactions gets **separate** rows per currency per bucket (e.g., two `cash_flow_view` rows for the same month, one per currency). This is the default, always-correct behavior requiring no conversion.
2. **Secondary, explicitly-derived strategy — normalized-currency views:** For users who want a single blended view, a **separately computed, clearly-labeled** projection (e.g., `cash_flow_view_normalized`, using a point-in-time exchange rate service, out of scope for MVP per `02_product_requirement_document.md`) would be added as an **additive** table, never by overwriting the primary currency-partitioned aggregates. This keeps the SRS's "raw amounts never lossy-converted" guarantee intact even as normalized views are introduced later.

---

## 15. Component Responsibilities (Schema-Level)

| Schema | Responsibility |
|---|---|
| `identity` | User identity and encrypted token custody |
| `ingestion` | Mailbox sync state and raw message capture lifecycle |
| `understanding` | Template definitions and extraction result history |
| `financialcore` | The canonical ledger, its override/audit trail, and categorization history — highest-integrity schema in the system |
| `budget` | Budget definitions and idempotent alert-firing state |
| `analytics` | Rebuildable, currency-partitioned read-optimized projections |
| `engagement` | Notification composition and two-tier delivery tracking |
| `administration` | Append-only, deletion-exempt audit trail and observability rollups |

---

## 16. Trade-offs

| Decision | Trade-off |
|---|---|
| Hybrid narrow-columns + `jsonb` payload for `financial_events` | Slightly more application-layer validation responsibility (per-type JSON schema enforcement) vs. a fully normalized table-per-type design, in exchange for a single, simple, always-consistent write path matching `04_domain_model.md`'s single-aggregate decision. |
| `category_assignments` as its own append-only table (not a column) | More storage and an extra join for "current category" reads, in exchange for full categorization audit history needed by Prompt Monitoring. |
| Currency-partitioned analytics tables (no default conversion) | Slightly more complex client-side rendering (a user might see two rows for one month), in exchange for never silently misrepresenting money — an acceptable and necessary trade-off for a financial product. |
| No cross-schema foreign keys | Loses database-enforced referential integrity across modules, in exchange for genuine schema independence and future extractability — mitigated by application-level validation and eventual-consistency-tolerant design (an orphaned reference is a monitorable data-quality metric, not a correctness catastrophe, since referenced aggregates are never hard-deleted, only archived). |

## 17. Advantages
- Every invariant from `04_domain_model.md` (INV-1 through INV-5) has an explicit, traceable physical enforcement mechanism in this schema — nothing is "enforced only in application code and hoped for."
- The Outbox/Inbox table pattern is consistent across every module, minimizing cognitive overhead when engineers move between modules.
- The `financial_events` schema is explicitly designed for future partitioning without requiring a breaking migration later.

## 18. Disadvantages
- The `jsonb` payload on `financial_events`, while pragmatic, requires ongoing discipline (per-type schema validation, documented in `04_domain_model.md`) to avoid becoming an unstructured junk drawer — this risk is explicitly flagged and must be enforced via application-layer validation tests, not assumed away by the schema alone.
- Lack of cross-schema FKs means some classes of data-integrity bugs (a `financial_events.user_id` pointing to a deleted `identity.users` row, in a hypothetical hard-delete scenario) are only caught by monitoring, not by the database — acceptable given the architecture's soft-delete/archive-only policy, but worth explicitly noting as a residual risk.

## 19. Future Improvements
- Introduce a dedicated append-only event-log table (separate from the transient `outbox_events`) if full historical event replay becomes a frequent operational need beyond Analytics rebuild (e.g., for a future BI/data-warehouse export pipeline) — noted in `06_architecture.md` Section 18 as well.
- Add the `cash_flow_view_normalized` (currency-converted) table set once a Phase 2/3 feature requires blended-currency reporting.
- Revisit `financial_events` partitioning activation threshold using real production volume data, per `23_scalability.md`.

## 20. Best Practices
- Every new `event_type` added to the taxonomy must have its `type_specific_payload` JSON schema documented in `04_domain_model.md` **before** any row using that type is written — schema-less "just add a field" additions are not permitted.
- Every migration (Goose) must be reviewed against this document's constraint list — a migration introducing a cross-schema FK should fail review automatically.
- `numeric(18,4)` (never `float`/`double precision`) is mandatory for all monetary columns across every schema, without exception.

## 21. Production Considerations
- The `idx_outbox_events_unrelayed` partial index strategy must be monitored for bloat under high-throughput conditions (frequent inserts/updates to `relayed`) — routine `VACUUM`/autovacuum tuning guidance belongs in `19_deployment.md`.
- `financial_events` partitioning (Section 7.1) should be activated proactively based on a monitored row-count/size threshold, not reactively after query performance has already degraded — a specific threshold and runbook is defined in `23_scalability.md`.
- Encrypted token columns (`oauth_tokens`) must be included in backup encryption-at-rest verification as part of routine security audits (`16_security.md`), since a backup is a copy of the same sensitive ciphertext and must be protected with equal rigor.

---

**Next Document:** `09_api_specification.md` — defines the OpenAPI-documented public API surface (endpoints, request/response DTOs, authentication, pagination, error formats) built directly on top of this schema and the module boundaries from `06_architecture.md`.
