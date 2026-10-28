# Observability
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `10_backend_blueprint.md`, `11_ai_blueprint.md`, `12_gmail_connector.md`, `13_email_parser.md`, `15_notification_system.md`, `16_security.md`
**Feeds Into:** `18_testing_strategy.md`, `19_deployment.md`, `22_risk_analysis.md`, `23_scalability.md`

---

## 1. Purpose

This document consolidates every observability requirement scattered across the prior thirteen documents into a single, authoritative specification: the complete Prometheus metric catalog, Grafana dashboard design, OpenTelemetry tracing conventions, structured logging conventions, and alerting/SLO definitions. Nearly every prior document included a phrase like "surfaced in `17_observability.md`" — this document is where those forward references are made concrete and complete.

Without this consolidation, observability requirements would remain scattered, easy to under-implement piecemeal, and impossible to review holistically for gaps. This document is also the direct basis for the production-readiness review gate referenced in `19_deployment.md`.

## 2. Scope

### 2.1 In Scope
- Full Prometheus metric catalog, organized by module, with type (counter/gauge/histogram), labels, and the specific document/section that originated the requirement.
- OpenTelemetry tracing span conventions and propagation rules.
- Structured logging field conventions.
- Grafana dashboard catalog (one per module, plus cross-cutting dashboards).
- Alerting rules and their severity/routing (feeding `14_workflow_engine.md`'s ops-alerting workflows).
- SLO definitions and error-budget philosophy.

### 2.2 Out of Scope
- Grafana dashboard JSON/PromQL query syntax itself (an implementation deliverable).
- Alertmanager routing configuration syntax (implementation deliverable; this document specifies routing *policy*, not YAML).
- Log aggregation infrastructure choice (e.g., Loki vs. ELK) — an implementation/deployment decision → `19_deployment.md`.

---

## 3. Architecture: Three Pillars, Consistently Applied

```
┌─────────────────────────────────────────────────────────────┐
│  Metrics (Prometheus)     │  Traces (OpenTelemetry)  │  Logs (slog)│
│  "What is the system's     │  "What happened to THIS    │  "What      │
│   aggregate behavior?"      │   specific request/task?"   │   exactly   │
│                             │                              │   happened?"│
└─────────────────────────────────────────────────────────────┘
                        │ correlated via
                        ▼
              requestId / traceId
     (present in every log line, every trace span,
      and returned in every API error envelope per
      09_api_specification.md §7.2)
```

**Design principle:** Every metric, span, and log line originates from the composition-root-wired instrumentation established in `06_architecture.md` Section 10 — no module hand-rolls its own observability approach. This document is the catalog of *what* gets instrumented; `06_architecture.md` established *how* (the wiring mechanism).

---

## 4. Prometheus Metric Catalog

### 4.1 Ingestion Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `ingestion_gmail_api_calls_total` | Counter | `method`, `status_code` | `12_gmail_connector.md` §8 |
| `ingestion_gmail_quota_units_consumed_total` | Counter | `account_id` (cardinality-bounded via bucketing, see §7) | `12_gmail_connector.md` §8 |
| `ingestion_watch_renewal_success_total` / `_failure_total` | Counter | — | `12_gmail_connector.md` §5.2 |
| `ingestion_watch_expiring_soon` | Gauge | — (count of accounts within 24h of expiry) | `12_gmail_connector.md` §5.2 |
| `ingestion_history_sync_fallback_total` | Counter | `reason` (`historyid_expired`, `downtime_recovery`) | `12_gmail_connector.md` §6.2 |
| `ingestion_poll_fallback_liveness` | Gauge (heartbeat timestamp) | — | `12_gmail_connector.md` §17 — "monitor the monitor" |
| `ingestion_messages_ingested_total` | Counter | `routing_decision` | `05_event_storming.md` Phase 2/3 |
| `ingestion_sync_lag_seconds` | Histogram | `account_id` (bucketed) | SRS §6.1 latency target |

### 4.2 Understanding Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `understanding_template_execution_total` | Counter | `sender_pattern`, `template_version`, `outcome` (`full_match`,`partial_match`,`failed`) | `13_email_parser.md` §8.1 |
| `understanding_template_failure_rate` | Gauge (computed) | `sender_pattern` | `13_email_parser.md` §8.1 |
| `understanding_ai_extraction_total` | Counter | `outcome` (`success`,`schema_invalid`,`reprompt_success`) | `11_ai_blueprint.md` §10 |
| `understanding_ai_confidence_distribution` | Histogram | `extraction_method` | `11_ai_blueprint.md` §6.3 |
| `understanding_ai_token_usage_total` | Counter | `direction` (`input`,`output`) | `11_ai_blueprint.md` §10, cost governance §11 |
| `understanding_ai_queue_depth` | Gauge | `queue_name` (`understanding:ai`) | `06_architecture.md` §6.5, H3 resolution |
| `understanding_ai_oldest_task_age_seconds` | Gauge | `queue_name` | `06_architecture.md` §6.5, H3 resolution |
| `understanding_ai_daily_budget_consumed_ratio` | Gauge | `scope` (`platform`,`per_user`) | `11_ai_blueprint.md` §11 |

### 4.3 Financial Core Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `financialcore_events_finalized_total` | Counter | `event_type`, `confidence_method` | `04_domain_model.md` §5.8 |
| `financialcore_events_pending_review_total` | Gauge | — | `04_domain_model.md` §5.8 |
| `financialcore_pending_review_sla_breach_total` | Counter | — | SRS §4.4 (7-day SLA) |
| `financialcore_overrides_submitted_total` | Counter | `field_name` | `04_domain_model.md` §5.5 |
| `financialcore_dedup_conflict_total` | Counter | — (ON CONFLICT DO NOTHING hits, i.e., successful idempotency saves) | `07_sequence_diagrams.md` Diagram 5 |
| `financialcore_confidence_calibration_override_rate` | Gauge | `confidence_bucket` | `11_ai_blueprint.md` §6.3 |

### 4.4 Budget Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `budget_threshold_crossed_total` | Counter | `threshold` | `04_domain_model.md` §9.3 |
| `budget_evaluation_duration_seconds` | Histogram | — | Operational baseline |

### 4.5 Analytics Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `analytics_projection_lag_seconds` | Histogram | `view_name` (`cash_flow`,`category_breakdown`,`merchant_ranking`,`trend`) | `06_architecture.md` §6.7, H6 resolution |
| `analytics_projection_checkpoint_age_seconds` | Gauge | `source_module` | `08_database_design.md` §11.5 |
| `analytics_query_duration_seconds` | Histogram | `endpoint` | SRS §6.1 |

### 4.6 Engagement Module

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `engagement_notifications_composed_total` | Counter | `notification_type` | `15_notification_system.md` §5 |
| `engagement_notifications_skipped_optout_total` | Counter | `notification_type` | `15_notification_system.md` §5.2 |
| `engagement_delivery_outcome_total` | Counter | `notification_type`, `tier` (`provider_accepted`,`delivered`,`read`,`failed`) | `15_notification_system.md` §7 |
| `engagement_whatsapp_messaging_tier_usage_ratio` | Gauge | — | `15_notification_system.md` §9 |
| `engagement_whatsapp_block_rate` | Gauge | — | `15_notification_system.md` §9, quality-rating risk |
| `engagement_reply_parse_outcome_total` | Counter | `outcome` (`button_match`,`text_pattern_match`,`unrecognized`) | `15_notification_system.md` §6 |

### 4.7 Administration & Cross-Cutting

| Metric | Type | Labels | Origin |
|---|---|---|---|
| `outbox_relay_lag_seconds` | Histogram | `module` | `06_architecture.md` §6.1 |
| `outbox_unrelayed_rows` | Gauge | `module` | `08_database_design.md` §4 |
| `asynq_dead_letter_total` | Counter | `queue_name` | `06_architecture.md` §6.4 |
| `asynq_task_retry_total` | Counter | `queue_name`, `task_type` | `06_architecture.md` §6.4 |
| `admin_ops_access_total` | Counter | `endpoint` | `16_security.md` §6.3 |
| `kms_operation_duration_seconds` / `kms_operation_failure_total` | Histogram/Counter | `operation` | `16_security.md` §4.3, §19 |
| `http_request_duration_seconds` | Histogram | `route`, `method`, `status_code` | Standard cross-cutting |

### 4.8 Cardinality Discipline

Per-account or per-sender-pattern labels (e.g., `ingestion_gmail_quota_units_consumed_total{account_id=...}`) are **never used directly with unbounded label cardinality** in production Prometheus — where per-entity granularity is genuinely needed for debugging (e.g., "which specific account is failing"), the metric is instead **bucketed** (e.g., a fixed-cardinality `account_id_bucket` derived via consistent hashing) for the always-on Prometheus metric, with true per-entity drill-down available via the corresponding Administration API endpoint (`09_api_specification.md` Section 6.6) or a trace query (Section 5), not via unbounded Prometheus label explosion. This is a mandatory review item for any new metric added to this catalog.

---

## 5. OpenTelemetry Tracing Conventions

### 5.1 Span Boundaries

A span is created at every module-boundary crossing established in `06_architecture.md` Section 10:
- HTTP handler entry (root span for synchronous requests).
- Asynq task handler entry (root span for asynchronous processing, **linked** to the originating request's trace if available via trace-context propagation through the Outbox `payload`, per Section 5.2).
- Every outbound external API call (Gmail, WhatsApp, LLM provider) as a child span, tagged with the external system name.
- Every repository call that crosses the module boundary conceptually (i.e., every `adapter/postgres` call) as a child span, tagged with the module and table/query name.

### 5.2 Trace Context Propagation Through the Outbox

Because the Outbox/Inbox pattern (`06_architecture.md` Section 6) deliberately decouples the triggering transaction from its asynchronous consumers, standard in-process trace propagation (context-passing) does not automatically span this boundary. To preserve the Vision's "a single Gmail message's journey is traceable end-to-end" requirement (`06_architecture.md` Section 10):

- The OpenTelemetry `trace_id`/`span_id` of the originating operation is **captured and stored as part of the `outbox_events.payload`** at write time.
- The Outbox Relay and the consuming Asynq task handler **extract and continue** this trace context (using OpenTelemetry's context propagation APIs) rather than starting a disconnected new trace — meaning a single trace can be queried end-to-end from "Gmail Pub/Sub webhook received" through "WhatsApp message delivered," even though the actual execution spans multiple queue hops and, potentially, multiple worker processes.

### 5.3 Span Attributes Convention

Every span includes, where applicable: `user.id`, `module.name`, `aggregate.id`, `aggregate.type` — following OpenTelemetry semantic-convention-style naming, kept consistent across every module per `20_coding_convention.md`'s tracing section (cross-referenced from there, defined authoritatively here).

**PII caution in spans:** Per `16_security.md` Section 7's classification tiers, spans **never** include Tier 1 (credentials) or raw Tier 4 (email content) as span attributes — only opaque identifiers (`user.id`, `aggregate.id`), never the values themselves. This is a mandatory review item for any new span attribute.

---

## 6. Structured Logging Conventions

- Every log line includes: `timestamp`, `level`, `module`, `requestId`/`taskId`, `traceId` (correlating directly to Section 5's traces), and a human-readable `message`.
- Per `16_security.md` Section 7, log lines **never** include Tier 1 or Tier 2 PII values directly (an email address is logged as a hashed/truncated reference or simply the opaque `user.id`, never the plaintext address) — this is enforced via a shared logging-helper convention (`20_coding_convention.md`) that structurally discourages passing raw PII fields into log calls.
- Log levels: `DEBUG` (local/staging only, disabled by default in production for volume/cost reasons), `INFO` (normal operational events — e.g., "sync completed," "notification delivered"), `WARN` (recoverable anomalies — e.g., "template partial match," "retry attempt N"), `ERROR` (failures requiring potential attention — e.g., "dead-lettered task," "schema-invalid AI response after re-prompt").

---

## 7. Grafana Dashboard Catalog

| Dashboard | Primary Audience | Key Panels |
|---|---|---|
| **Platform Overview** | On-call/leadership | Aggregate request rate/latency/error rate, Outbox relay lag across all modules, active incident count |
| **Connector Health** | Ops, FR-ADMIN-01 | Per-bucketed-account sync lag, watch renewal success rate, quota consumption trend, poll-fallback liveness heartbeat |
| **Parser Monitoring** | Ops/template maintainers, FR-ADMIN-02 | Per-template failure rate (with the H3 spike-alert threshold line overlaid), template version distribution, fallback-to-AI rate |
| **Prompt Monitoring** | AI/ML engineers, FR-ADMIN-03 | Confidence distribution per prompt version, calibration (override-rate by confidence bucket, per `11_ai_blueprint.md` §6.3), token usage/cost trend, schema-validation failure rate |
| **Financial Core Health** | Engineering | Finalization rate, pending-review queue depth and SLA-breach count, dedup-conflict rate (idempotency working-as-intended signal), override submission rate by field |
| **Analytics Freshness** | Engineering | Projection lag per view (H6), checkpoint age per source module |
| **Engagement / Notification** | Product, Ops | Delivery outcome funnel (composed → provider-accepted → delivered → read), opt-out rates, WhatsApp messaging-tier usage and block-rate trend (quality-rating risk) |
| **Security & Compliance** | Security, Ops | `ops`-role access log volume, KMS operation latency/failure rate, GDPR erasure-cascade completion time distribution |
| **Cost & Capacity** | Engineering leadership | AI token spend trend, Gmail quota headroom, WhatsApp messaging-tier headroom, database size/partition growth |

---

## 8. Alerting Rules & Severity

| Alert | Condition | Severity | Routes To (via `14_workflow_engine.md`) |
|---|---|---|---|
| Template failure-rate spike | `understanding_template_failure_rate > 5%` (trailing 1h) | High | Parser-maintenance Slack channel (auto-enriched, per `14_workflow_engine.md` §4.2) |
| AI queue backpressure | `understanding_ai_queue_depth` or `understanding_ai_oldest_task_age_seconds` exceeds threshold | High | On-call Slack/PagerDuty |
| Watch renewal failure | `ingestion_watch_renewal_success_total` stagnant while `_failure_total` increments for an account | Medium | Connector-health Slack channel |
| Poll-fallback liveness stale | `ingestion_poll_fallback_liveness` heartbeat older than 2x expected interval | Critical | PagerDuty (this is the H2 safety-net's own failure — the most severe class per `12_gmail_connector.md` §17) |
| Outbox relay lag | `outbox_relay_lag_seconds` P95 exceeds SLO (Section 9) | Medium | On-call Slack |
| Pending-review SLA breach | `financialcore_pending_review_sla_breach_total` increments | Low (per-user), Medium (rate spike) | Product/ops digest |
| WhatsApp block-rate rise | `engagement_whatsapp_block_rate` trending upward past a defined threshold | High | Product + Engineering (per `15_notification_system.md` §9's cross-functional note) |
| KMS operation failure | `kms_operation_failure_total` increments | Critical | PagerDuty |
| GDPR erasure approaching deadline | Purge request age approaching the jurisdiction-mandated window without completion (`16_security.md` §19) | Critical | PagerDuty + Compliance contact |
| Dead-letter queue growth | `asynq_dead_letter_total` increments beyond a low baseline | Medium | On-call Slack |

**Severity-to-routing mapping** (Critical → PagerDuty page; High → Slack with escalation if unacknowledged per `14_workflow_engine.md` Section 4.1; Medium/Low → digest/dashboard visibility) is itself implemented as an n8n routing workflow, per that document's sanctioned scope — Prometheus/Alertmanager computes the *condition*, n8n handles the *routing/presentation*, consistent with the boundary established there.

---

## 9. SLO Definitions & Error Budget Philosophy

| SLI | SLO Target | Source Requirement |
|---|---|---|
| Finalization latency (template path) | P95 ≤ 2 min | SRS §6.1 |
| Finalization latency (AI path) | P95 ≤ 5 min | SRS §6.1 |
| Analytics query latency | P95 ≤ 500ms | SRS §6.1 |
| Analytics projection freshness | P95 ≤ [tuned value, TBD via production data per `06_architecture.md` §6.7] | SRS §6.1 (extension) |
| API availability | ≥ 99.9% monthly | SRS §6.2 |
| WhatsApp delivery success (excl. opt-out/block) | ≥ 99% | `02_product_requirement_document.md` §14 |
| Watch renewal reliability | ≥ 99.99% | SRS §6.2 |
| GDPR erasure completion | 100% within regulatory window | `16_security.md` §9.2 |

**Error budget philosophy:** SLOs with an explicit numeric target (not the two 100%-required compliance/financial-integrity ones) carry a conventional error-budget model — burn-rate alerting (fast-burn vs. slow-burn windows) is the intended alerting *strategy* for these SLOs, refining the simpler threshold-based alerts in Section 8 as the platform matures past MVP; simple threshold alerting is the launch-appropriate starting point, with burn-rate refinement flagged as a Future Improvement (Section 16) rather than a launch blocker.

**Note on the two 100%-required items:** GDPR erasure completion and (implicitly) the exactly-once ledger guarantee are **not conventional SLOs with an acceptable error budget** — a single miss is a compliance/correctness incident, not a budget-consuming event. This distinction must be preserved in how these two are dashboarded and alerted (Section 8's `Critical` severity, not a burn-rate-tolerant SLO), and must never be silently reclassified as an ordinary SLO with a tolerable miss rate.

---

## 10. Detailed Explanation: Why Trace Context Must Explicitly Cross the Outbox Boundary

It would be simpler to treat each queue hop as the start of a new, disconnected trace — many systems do exactly this, accepting fragmented observability across async boundaries as an unavoidable cost of the Outbox pattern. This document explicitly rejects that shortcut (Section 5.2) because:

1. **The platform's own architecture is built around asynchronous, multi-hop processing as the *normal* case**, not an edge case (`06_architecture.md` Section 6) — a Gmail message's journey to a delivered WhatsApp notification crosses at minimum four to five queue hops (ingestion → understanding → financialcore → analytics → engagement) even in the simplest happy path. If trace context fragmented at every hop, debugging any real production issue would require manually stitching together five to six separate, disconnected trace views by correlating timestamps and IDs — exactly the kind of debugging friction OpenTelemetry exists to eliminate.
2. **This is directly foreshadowed and required by `06_architecture.md` Section 10's original claim** ("even though it's in-process today... deliberately designed to remain valid once/if modules are extracted") — that claim is only true if trace propagation is explicitly engineered to survive the transport boundary now, while it's cheap to build correctly, rather than retrofitted later when the Outbox's payload schema is already in wide production use and harder to extend.
3. **The cost is small and one-time:** storing a `trace_id`/`span_id` pair in the Outbox payload (Section 5.2) is a negligible addition to every event's schema, paid once in the schema design, versus the recurring, compounding cost of fragmented traces during every future production incident investigation.

---

## 11. Component Responsibilities

| Component | Responsibility |
|---|---|
| Composition-root instrumentation (`06_architecture.md` §10) | Wires the shared OpenTelemetry SDK, Prometheus registry, and slog handler consistently across every module |
| Outbox Relay / Inbox Guard | Trace-context capture and continuation across the async boundary (Section 5.2) |
| Prometheus | Metric scraping and storage; alert-rule evaluation |
| Grafana | Dashboard rendering per Section 7's catalog |
| Alertmanager | Alert routing to n8n webhooks per Section 8 |
| n8n (per `14_workflow_engine.md`) | Alert presentation/escalation, never condition computation |

---

## 12. Trade-offs

| Decision | Trade-off |
|---|---|
| Bucketed (not raw) per-account/per-sender Prometheus labels | Less immediate per-entity drill-down directly in Grafana, in exchange for bounded, production-safe metric cardinality at scale (SRS §6.5's 100,000+ mailbox target). |
| Explicit trace-context propagation through the Outbox | A small, permanent addition to every Outbox event's payload schema, in exchange for genuinely unified, debuggable end-to-end traces across the platform's inherently asynchronous architecture. |
| Threshold alerting at launch, burn-rate alerting deferred | Less refined alert precision initially (more potential for either noisy or slow-to-fire alerts on gradually-degrading SLOs), in exchange for not over-engineering alerting sophistication before there's enough production traffic data to tune burn-rate windows meaningfully. |

## 13. Advantages
- This document closes every forward reference to `17_observability.md` scattered across the prior thirteen documents, meaning nothing in the production-readiness picture is left as an unfulfilled promise.
- The metric catalog's explicit "Origin" column (Section 4) makes it trivial to verify, document by document, that every stated monitoring requirement in the series has a concrete corresponding metric — a direct traceability mechanism mirroring the `FR-*` traceability established in `02_product_requirement_document.md`.
- Cross-boundary trace propagation (Section 5.2) gives the platform genuinely rare, high-value end-to-end debuggability for a system this architecturally asynchronous.

## 14. Disadvantages
- The sheer size of the metric catalog (Section 4) represents meaningful upfront instrumentation work before any dashboard delivers value — an accepted cost given the "never simplify the design" directive, but real effort nonetheless.
- Deferring burn-rate alerting (Section 9) means the launch-era alerting will be somewhat cruder (more prone to either false positives on transient blips or slower detection of slow-burn degradation) than a fully mature SRE practice would prefer — an explicit, documented, and revisited-later trade-off, not an oversight.

## 15. Future Improvements
- Implement multi-window, multi-burn-rate alerting for the numeric-target SLOs in Section 9 once sufficient production traffic history exists to tune the windows meaningfully (a standard SRE maturity progression, appropriately deferred past MVP).
- Consider exposing a subset of the Connector Health and Engagement dashboards (Section 7) as user-facing status indicators (e.g., "your Gmail sync is healthy" in the app), extending observability data's value beyond internal ops use.
- Introduce automated anomaly detection (beyond static thresholds) for metrics like `engagement_whatsapp_block_rate` and `understanding_template_failure_rate`, where a static threshold may be either too sensitive or too coarse across the full range of production traffic patterns as the platform scales.

## 16. Best Practices
- Every new metric added to any module must be added to this document's Section 4 catalog (with its originating document/section noted) before or alongside implementation — this document must never silently drift out of sync with the actual instrumented system.
- Any new span attribute must be checked against `16_security.md` Section 7's PII tiers (Section 5.3) before being added — this is a mandatory code-review checklist item, not a one-time policy statement.
- Dashboard panels should be provisioned as code (JSON checked into the repository, per `06_architecture.md` Section 10) — no manually-clicked-together, undocumented production dashboard should be considered authoritative.

## 17. Production Considerations
- The full metric catalog (Section 4) and dashboard catalog (Section 7) together constitute the checklist for the production-readiness review gate referenced in `19_deployment.md` — launch should not proceed with unimplemented items from either catalog without an explicit, documented, time-bounded exception.
- Alert fatigue must be actively monitored post-launch (are Section 8's alerts firing at an actionable rate, or are they noisy?) — this is itself a metric worth tracking (alert-to-actioned ratio) and revisiting within the first operational month, mirroring `02_product_requirement_document.md`'s directive to revisit the confidence threshold using real data.
- The two 100%-required compliance/correctness SLOs (Section 9's closing note) must be reviewed at every architecture or schema change proposal as a standing agenda item, since they represent the platform's hardest non-negotiable guarantees and are the items most likely to be inadvertently weakened by an otherwise-reasonable-seeming future change.

---

**Next Document:** `18_testing_strategy.md` — specifies the full testing pyramid (unit, integration, contract, end-to-end), the AI-specific evaluation-set testing approach referenced in `11_ai_blueprint.md`, the mandatory GDPR-cascade and idempotency test scenarios flagged throughout the series, and the CI pipeline's test-gating structure.
