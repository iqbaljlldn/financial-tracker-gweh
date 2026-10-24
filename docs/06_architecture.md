# Architecture
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`, `03_software_requirement_specification.md`, `04_domain_model.md`, `05_event_storming.md`
**Feeds Into:** `07_sequence_diagrams.md`, `08_database_design.md`, `09_api_specification.md`, `10_backend_blueprint.md`, `21_project_structure.md`

---

## 1. Purpose

This document defines the **technical architecture** of the platform: how the bounded contexts from `04_domain_model.md` and the process timeline from `05_event_storming.md` are realized as a deployable system using the mandated stack (Go 1.25.2, chi, PostgreSQL, sqlc, Goose, Redis, Asynq, n8n, WhatsApp API, OpenTelemetry, Prometheus, Grafana, Podman, Traefik, GitLab CI).

This is the document where **"why this technology, arranged this way"** is justified. It resolves the eight hotspots (H1–H8) registered in `05_event_storming.md`, defines the Clean Architecture layering and module boundary rules, specifies the Outbox/Inbox event-driven mechanism that makes exactly-once *effect* possible atop at-least-once *transport*, and lays out the worker topology that processes the event timeline asynchronously and reliably.

## 2. Scope

### 2.1 In Scope
- Overall system topology (modular monolith, deployment units, module boundaries).
- Clean Architecture layering applied to this domain.
- Event-driven mechanics: Outbox pattern, Inbox pattern, Asynq queue topology.
- Technology-to-component mapping and justification.
- Resolution of hotspots H1–H8.
- Cross-cutting concerns architecture (observability, security boundary, configuration).
- CQRS boundary between Financial Core (write) and Analytics (read).
- Failure domains and degradation behavior.

### 2.2 Out of Scope
- Concrete schema DDL → `08_database_design.md`
- API contracts → `09_api_specification.md`
- Package-level Go project layout → `21_project_structure.md`
- Detailed sequence-level request flows → `07_sequence_diagrams.md`
- AI prompt/model specifics → `11_ai_blueprint.md`

---

## 3. Architecture Style Decision

### 3.1 Modular Monolith, Microservice-Ready

**Decision:** The platform ships as a **single deployable Go binary** (the modular monolith), internally organized into strictly bounded modules that mirror the bounded contexts from `04_domain_model.md` one-to-one. Each module is built as if it were a future microservice: it owns its persistence schema (via PostgreSQL schemas, not merely conventions), communicates with other modules only through defined Go interfaces and domain events, and has no compile-time dependency on another module's internal types.

**Why not microservices at launch:**
- Operational overhead (service discovery, distributed tracing complexity, network-partition handling, N deployment pipelines) is not justified at MVP scale (thousands, not millions, of users).
- A modular monolith with enforced boundaries gets ~80% of microservices' maintainability benefit (independent reasoning, testability, clear ownership) at a fraction of the operational cost.
- The event-driven mechanism designed in Section 6 (Outbox/Inbox via Asynq+Redis+Postgres) is **transport-agnostic in shape** — the same domain event contracts used in-process today can be republished to a message broker (e.g., NATS, Kafka) later without changing domain/service code, only the transport adapter.

**Why not a "big ball of mud" monolith either:**
- Without enforced module boundaries, a monolith inevitably accretes cross-module database joins and implicit coupling that make future extraction effectively impossible. This architecture makes boundary violations a **compile-time or CI-time failure**, not a code-review nicety (Section 5.4).

### 3.2 Clean Architecture Applied

Each module internally follows Clean Architecture's dependency rule: dependencies point inward, toward the domain.

```
┌─────────────────────────────────────────────────────────────┐
│  Interface Adapters (per module)                              │
│  - HTTP handlers (chi)     - Asynq task handlers               │
│  - Gmail API client        - WhatsApp API client                │
│  - LLM Provider client     - sqlc-generated repository impls    │
├─────────────────────────────────────────────────────────────┤
│  Application / Use Case Layer (per module)                    │
│  - Command handlers (e.g., LinkGmailAccountHandler)            │
│  - Query handlers (e.g., GetCashFlowViewHandler)                │
│  - Policy handlers (e.g., OnMessageIngestedHandler)             │
├─────────────────────────────────────────────────────────────┤
│  Domain Layer (per module)                                     │
│  - Aggregates, Entities, Value Objects (from 04_domain_model)   │
│  - Domain Events                                                │
│  - Domain Services (invariant enforcement)                      │
└─────────────────────────────────────────────────────────────┘
```

**Dependency Rule enforced:** The Domain layer has zero imports from Application or Interface Adapters. The Application layer depends only on Domain layer types and on repository/gateway **interfaces** defined in the Domain/Application layer (not their sqlc/HTTP-client implementations). Interface Adapters implement those interfaces and depend inward. This is enforced via Go's package structure and import-boundary linting (`golangci-lint` custom rule / `depguard`) detailed in `21_project_structure.md` and `20_coding_convention.md`.

**Why Clean Architecture here specifically:** The extraction pipeline (Section 5) has genuinely swappable infrastructure at every edge — the LLM provider, the Gmail connector (vs. future Outlook/IMAP), the WhatsApp provider. Clean Architecture's inward-pointing dependency rule is what makes "swap the LLM provider without touching domain logic" a real, low-risk operation rather than an aspiration.

---

## 4. Module Map (Bounded Context → Deployable Module)

| Module (Go package root) | Bounded Context | Owns Schema |
|---|---|---|
| `identity` | Identity & Access | `identity` (Postgres schema) |
| `ingestion` | Ingestion (Gmail Connector) | `ingestion` |
| `understanding` | Understanding (Parsing + AI Extraction) | `understanding` |
| `financialcore` | Financial Core (Ledger) | `financialcore` |
| `budget` | Budget | `budget` |
| `analytics` | Analytics (CQRS read side) | `analytics` |
| `engagement` | Engagement (Notification) | `engagement` |
| `administration` | Administration | `administration` |

Each module is a top-level Go package with its own `domain/`, `application/`, `adapter/` sub-packages (full layout in `21_project_structure.md`). Each module owns exactly one PostgreSQL schema; **no module's SQL (via sqlc) may reference another module's schema directly** — this is the concrete enforcement mechanism for the Domain Model's cross-context interaction rule (Section 13 of `04_domain_model.md`).

---

## 5. Module Boundary Enforcement

### 5.1 Allowed Cross-Module Interaction

1. **Domain Events** (in-process pub/sub today, broker-ready contract) — the primary mechanism, matching the Policy list from `05_event_storming.md`.
2. **Published Interfaces** — a module may expose a narrow Go interface (e.g., `identity.TokenProvider`) that another module's Application layer depends on, injected via dependency injection at startup (Section 8). This is used sparingly, for synchronous needs like "Ingestion needs a valid access token right now to call Gmail API" — not for general data access.
3. **Read Models (Analytics only)** — Analytics is the sole module permitted to build read-optimized projections from another module's (Financial Core's) domain events; this is the CQRS read-side, not a general pattern other modules should imitate.

### 5.2 Forbidden Interaction

- No module imports another module's `domain` or `adapter` packages directly (only narrowly published interfaces, per 5.1.2).
- No module's SQL queries join across schemas.
- No module holds a foreign key constraint into another module's schema at the database level (referential integrity across modules is enforced at the application level via ID validation, not database FK, precisely to keep schemas independently extractable — detailed further in `08_database_design.md`).

### 5.3 Enforcement Mechanism

- **CI-time boundary check:** A GitLab CI job runs a Go import-graph linter (e.g., `go-arch-lint` or an equivalent custom tool configured against this module map) that fails the pipeline on any forbidden cross-module import.
- **Schema-level enforcement:** Distinct PostgreSQL schemas per module, with the database role used by sqlc-generated code for each module granted access **only** to its own schema — a defense-in-depth measure so an accidental cross-schema query fails at the database permission level even if it somehow passed code review.

### 5.4 Why This Matters More Here Than in a Typical CRUD App

This platform's stated evolution path (Vision, Section 8) explicitly requires modules to become independently extractable services under scale pressure (e.g., the Understanding module's AI extraction load may need to scale independently of the Ingestion module's I/O-bound Gmail polling). Boundary violations discovered at CI time are cheap to fix; boundary violations discovered during a service extraction 18 months from now are a multi-sprint remediation project. This is a direct, load-bearing engineering consequence of the Vision's non-negotiable principles — not an academic purity exercise.

---

## 6. Event-Driven Mechanics: Outbox, Inbox, and Queue Topology

This section is the architectural resolution of the SRS's hardest requirement: **exactly-once effect atop at-least-once transport** (SRS Section 6.3), and directly implements the Policy list from `05_event_storming.md`.

### 6.1 The Outbox Pattern (Reliable Event Publishing)

**Problem:** When a module's command handler both (a) persists a state change and (b) needs to trigger a reactive policy (e.g., persist `FinancialEvent` as finalized AND trigger `ProjectAnalyticsReadModels`), a naive "write to DB, then publish to queue" sequence has a race: if the process crashes between the DB write and the queue publish, the event is silently lost — analytics would never update, budgets would never evaluate.

**Solution:** Every command handler that changes aggregate state **and** needs to emit a domain event does so within a **single PostgreSQL transaction** that writes both the aggregate state change and a row into that module's `outbox_events` table. A separate, lightweight **Outbox Relay** worker polls `outbox_events` (ordered, unprocessed rows) and publishes each as an Asynq task, marking the outbox row as `relayed` only after successful enqueue.

```
┌─────────────────────────────────────────────┐
│  Command Handler (e.g., FinalizeFinancialEvent)│
│                                                  │
│  BEGIN TRANSACTION                              │
│    UPDATE financial_events SET status=...       │
│    INSERT INTO outbox_events (event_type,       │
│      payload, aggregate_id, created_at)          │
│  COMMIT                                          │
└──────────────────┬──────────────────────────┘
                    │ (polled by)
                    ▼
┌─────────────────────────────────────────────┐
│  Outbox Relay Worker                            │
│  - SELECT ... FROM outbox_events                │
│    WHERE relayed = false ORDER BY created_at    │
│    FOR UPDATE SKIP LOCKED                        │
│  - Enqueue as Asynq task                         │
│  - UPDATE outbox_events SET relayed = true       │
└──────────────────┬──────────────────────────┘
                    ▼
              Redis (Asynq queue)
```

**Why this specific mechanism (DB-transactional outbox, not a distributed transaction or dual-write):** A distributed transaction (2PC) across Postgres and Redis is operationally fragile and not natively supported cleanly across these two systems. The transactional-outbox pattern trades a small publish latency (bound by the relay's poll interval, sub-second in practice) for a guarantee that is otherwise very hard to achieve: **the event is published if and only if the state change committed.**

### 6.2 The Inbox Pattern (Idempotent Consumption)

**Problem:** Asynq (backed by Redis) provides **at-least-once delivery** to task handlers. A handler may be invoked twice for the same task (e.g., worker crash after processing but before acking, retry after a transient error that actually succeeded). Without protection, this would violate INV-3 from `04_domain_model.md` (no duplicate `FinancialEvent`) and the idempotent-alerting requirement for budgets.

**Solution:** Every Asynq task handler that has a **side effect with a natural idempotency key** (Gmail `message_id`, a specific `(budget_id, period, threshold)` tuple, a specific `notification_job_id`) first checks/inserts into an `inbox_processed` table (or, for the Financial Core specifically, relies on the domain-level unique constraint from INV-3 directly, which is the more precise mechanism there) within the **same transaction** as the side-effecting write. A duplicate delivery either fails the unique-constraint insert (treated as success, per SRS-FR-PARSE-02's stated error handling) or is short-circuited by an inbox-table check before any work is even attempted (used for handlers whose side effects aren't naturally idempotent, e.g., an external API call to WhatsApp).

```
┌─────────────────────────────────────────────┐
│  Asynq Task Handler (e.g., DeliverViaWhatsApp) │
│                                                  │
│  BEGIN TRANSACTION                              │
│    SELECT 1 FROM inbox_processed                │
│      WHERE task_id = ? FOR UPDATE                │
│    -- if found: COMMIT and return (no-op)        │
│    -- if not found:                              │
│      INSERT INTO inbox_processed (task_id, ...)  │
│      [perform side effect / call external API]   │
│  COMMIT                                          │
└─────────────────────────────────────────────┘
```

**Note on external-call ordering:** For handlers calling an external, non-transactional API (WhatsApp, Gmail, LLM provider), the inbox-check happens *before* the external call where possible; where the external call itself is naturally idempotent (e.g., Gmail API reads), the check is a lighter-weight guard rather than a strict pre-condition. This distinction is detailed per-handler in `10_backend_blueprint.md`.

### 6.3 Asynq Queue Topology

Asynq (Redis-backed) is used for all asynchronous task execution — both the Outbox Relay's publish target and the general background job runtime (parsing, AI extraction, projection, notification delivery).

| Queue | Purpose | Priority | Concurrency Model |
|---|---|---|---|
| `ingestion:critical` | Gmail watch renewal, Pub/Sub-triggered incremental sync | High | Low concurrency, latency-sensitive |
| `ingestion:bulk` | Initial full sync message enumeration | Low | High concurrency, throughput-oriented |
| `understanding:template` | Template/regex parsing | High | High concurrency (cheap, fast) |
| `understanding:ai` | AI extraction/categorization LLM calls | Medium | **Rate-limited concurrency**, bounded by LLM provider quota (resolves H3 — see Section 6.5) |
| `financialcore:finalize` | Confidence gating, finalization, override application | High | Medium concurrency, strict ordering per aggregate not required (per-event independence) |
| `analytics:project` | Read-model projection | Medium | Medium concurrency, can lag slightly under load (bounded per H6, monitored) |
| `budget:evaluate` | Budget threshold evaluation | Medium | Medium concurrency |
| `engagement:compose` | Notification composition | Medium | Medium concurrency |
| `engagement:deliver` | WhatsApp delivery | High | Rate-limited concurrency, bounded by WhatsApp Business API rate limits |
| `dead-letter` | Exhausted-retry landing zone (per-source-queue) | N/A | Manual/administrative inspection only |

**Why separate queues per concern (not one general queue):** Asynq supports weighted multi-queue processing, allowing independent concurrency and priority tuning per concern. This is essential because `understanding:ai` and `engagement:deliver` have **hard external rate limits** that must not be allowed to starve or be starved by `understanding:template`'s much higher natural throughput. A single shared queue would require complex per-task-type throttling logic; separate queues let Asynq's built-in weighted worker pool handle this declaratively.

### 6.4 Retry & Dead-Letter Policy

- Default: exponential backoff, base 10s, multiplier 2, max 5 attempts, ±20% jitter — configurable per queue via Viper (Section 9).
- Non-retryable errors (schema validation failures, 4xx from external APIs indicating a permanent client-side problem) are classified explicitly in each handler and routed directly to `dead-letter` without exhausting the retry budget, to avoid wasting time/quota on a failure mode retries cannot fix.
- Every `dead-letter` landing is a mandatory audit-logged event (`administration` module subscribes to Asynq's dead-letter hook) — satisfying FR-ADMIN-02/03's monitoring requirement at the infrastructure level.

### 6.5 Resolving Hotspot H3 (AI Extraction Fallback Storm)

The `understanding:ai` queue has a **hard concurrency ceiling** matched to the LLM provider's rate limit (configured, not hardcoded). When template-parser failures spike for a high-volume sender (per `05_event_storming.md`'s H3), the flood of `InvokeAIExtractor` tasks queues up rather than overwhelming the provider — Asynq's queue depth for `understanding:ai` becomes a direct, monitorable backpressure signal. A Prometheus alert (`17_observability.md`) fires when this queue's depth or age-of-oldest-task exceeds a threshold, correlated with a per-template failure-rate spike from Parser Monitoring, giving on-call engineers a clear signal to prioritize a template fix rather than the system silently degrading into cost overrun or provider throttling. This is the concrete architectural resolution promised in `05_event_storming.md`.

### 6.6 Resolving Hotspot H2 (Silent Sync Gap)

As specified in `05_event_storming.md`, ingestion uses a **dual-trigger** design:
1. **Push-driven:** Pub/Sub webhook → `ingestion:critical` queue → `RunIncrementalSync`.
2. **Poll-driven fallback:** A scheduled Asynq periodic task (via `asynq.PeriodicTaskManager` or an equivalent cron-scheduled enqueue, e.g., every 15 minutes per `GmailAccount`) independently triggers `RunIncrementalSync` regardless of push health.

Because `RunIncrementalSync` is itself idempotent (bounded by the stored `historyId` and the Inbox-pattern dedup on `gmail_message_id`), running it redundantly (once from push, once from poll) is safe and cheap — the second invocation typically finds nothing new. This closes the gap without requiring complex push-health detection logic.

### 6.7 Resolving Hotspot H6 (Analytics Projection Lag as a Distinct Metric)

The Outbox Relay's publish latency, the `analytics:project` queue's processing latency, and end-to-end "event finalized → read model updated" latency are each captured as distinct Prometheus histograms (detailed in `17_observability.md`). A dedicated freshness metric — `analytics_projection_lag_seconds` — is exposed per user-read-model type, giving an explicit, monitorable answer to "how stale can the dashboard be" rather than an implicit assumption.

---

## 7. CQRS Boundary: Financial Core (Write) vs. Analytics (Read)

Financial Core owns the single write path for `FinancialEvent`. Analytics owns zero write authority over that data and instead:

1. Subscribes to `FinancialEventFinalized`, `FinancialEventOverridden`, `FinancialEventArchived`, `CategoryAssignmentChanged` (via the Outbox/queue mechanism, Section 6).
2. Projects these into its own schema's read-optimized tables (denormalized, aggregation-friendly — detailed in `08_database_design.md`), owned exclusively by the `analytics` module.
3. Serves all query-side API traffic (`09_api_specification.md`'s analytics endpoints) from these projections, never from Financial Core's write-side tables directly.

**Why this specific CQRS scope (not full CQRS everywhere):** Full CQRS (separate read/write models for every module) would be over-engineering for modules with simple, low-volume read patterns (e.g., Budget definitions are read exactly as they are written — no aggregation needed). CQRS is applied **precisely where the read pattern (aggregation, trend computation, ranking) is structurally different from the write pattern (single-event append)** — which is uniquely true of Analytics. This is a targeted application of the pattern, in line with the Vision's "use CQRS where beneficial" instruction rather than "use CQRS everywhere."

**Rebuildability requirement:** Per SRS Section 6.3, Analytics read models must be fully reconstructable by replaying Financial Core's event history from scratch. This means the Outbox events are not transient — they (or a durable event log derived from them) must be retained long enough to support a full read-model rebuild, addressed at the storage-retention level in `08_database_design.md`.

---

## 8. Dependency Injection & Application Composition

- **DI approach:** Manual, explicit constructor-based dependency injection at the application's `main.go` composition root (no reflection-based DI framework) — chosen for compile-time safety, explicitness, and alignment with idiomatic Go practice, and because the module count (8) is small enough that manual wiring remains manageable and highly legible.
- Each module exposes a `NewModule(deps...) *Module` constructor that wires its own Domain → Application → Adapter layers internally and returns a small public surface (its HTTP route registrar, its Asynq handler registrar, and any explicitly published cross-module interface implementations per Section 5.1.2).
- The composition root wires cross-module interfaces (e.g., `identity.TokenProvider` implementation into `ingestion`'s constructor) explicitly, keeping the full dependency graph visible in one place rather than hidden behind a container.

---

## 9. Configuration Architecture

- **Viper** is used for layered configuration: defaults → config file (`config.yaml`, environment-specific) → environment variables (highest precedence, used for secrets in production, injected via the deployment platform's secret manager, not committed config).
- Each module defines its own typed configuration struct (e.g., `understanding.Config{AIProviderBaseURL, AIRateLimitPerSecond, ConfidenceThreshold}`), validated at startup via `go-playground/validator`, failing fast on invalid configuration rather than at first use.
- Configuration for external system credentials (Gmail OAuth client secret, WhatsApp API token, LLM provider API key) is never stored in the config file directly — Viper reads these from environment variables sourced from a secrets manager at deploy time (detailed in `16_security.md`, `19_deployment.md`).

---

## 10. Observability Architecture (Cross-Cutting)

- **Logging:** Structured logging via `slog`, with a consistent set of contextual fields injected at the composition root and propagated via `context.Context` (request ID, user ID where applicable, module name, aggregate ID where applicable) — full convention in `20_coding_convention.md`.
- **Tracing:** OpenTelemetry instrumentation at every module boundary crossing (HTTP handler entry, Asynq task handler entry, outbound external API calls) so a single Gmail message's journey through Ingestion → Understanding → Financial Core → Analytics → Engagement is traceable end-to-end as one distributed trace, even though it's in-process today — this is deliberately designed to remain valid once/if modules are extracted into separate services.
- **Metrics:** Prometheus metrics exposed per module (queue depth, task processing latency/error rate, external API call latency/error rate, confidence-score distributions, projection lag) — full catalog in `17_observability.md`.
- **Dashboards:** Grafana, provisioned as code (dashboard JSON checked into the repo), avoiding manual dashboard drift.

This is elaborated fully in `17_observability.md`; this section establishes only that observability is a **first-class architectural concern wired at the composition root**, not an afterthought bolted on per handler.

---

## 11. Deployment Topology (High-Level; Full Detail in `19_deployment.md`)

```
                         ┌─────────────┐
                         │   Traefik    │  (Reverse Proxy / TLS termination)
                         └──────┬──────┘
                                │
                  ┌─────────────┴─────────────┐
                  ▼                            ▼
          ┌───────────────┐           ┌───────────────┐
          │  API Instance   │  ...      │  API Instance   │   (horizontally scaled,
          │  (Go binary,    │           │  (Go binary,    │    stateless HTTP layer)
          │   chi router)   │           │   chi router)   │
          └───────┬───────┘           └───────┬───────┘
                  │                            │
                  └─────────────┬──────────────┘
                                ▼
                       ┌─────────────────┐
                       │   PostgreSQL      │  (primary + read replica,
                       │   (per-module      │   schema-per-module)
                       │   schemas)         │
                       └─────────────────┘
                                ▲
                                │
          ┌─────────────────────┼─────────────────────┐
          ▼                     ▼                       ▼
  ┌───────────────┐   ┌───────────────┐       ┌───────────────┐
  │ Worker Instance │   │ Worker Instance │  ...  │ Worker Instance │  (horizontally scaled,
  │ (Asynq consumer,│   │ (Asynq consumer,│       │ (Asynq consumer,│   process the queues
  │  same Go binary,│   │  same Go binary,│       │  same Go binary,│   from Section 6.3)
  │  worker mode)    │   │  worker mode)    │       │  worker mode)    │
  └───────┬───────┘   └───────┬───────┘       └───────┬───────┘
          │                     │                       │
          └─────────────┬───────┴───────────────────────┘
                        ▼
                 ┌─────────────┐
                 │    Redis      │  (Asynq queue backend + caching)
                 └─────────────┘

                 ┌─────────────┐
                 │    n8n        │  (Workflow automation — see 14_workflow_engine.md
                 │              │   for its precise, bounded role)
                 └─────────────┘
```

**Note on the single-binary, dual-mode deployment:** The same compiled Go binary runs in either `api` mode (HTTP server via chi) or `worker` mode (Asynq consumer), selected by a startup flag/environment variable. This keeps the build/release pipeline simple (one artifact) while allowing independent horizontal scaling of API vs. worker instances — a practical middle ground between "one process does everything" and "N independently built services," appropriate for a modular monolith.

**Role of n8n:** n8n is scoped deliberately narrowly — it is **not** the core event-processing engine (that is Asynq, for reliability/idempotency/observability reasons detailed above). n8n's role is confined to lower-stakes, operationally-convenient automation: e.g., ops-facing alerting workflows, semi-automated sender-template onboarding assistance, or ad hoc administrative automations that benefit from a visual workflow builder. This boundary is fully specified in `14_workflow_engine.md` and must not be blurred — no core financial data processing logic (extraction, finalization, budget evaluation) is permitted to live in an n8n workflow, precisely because it would bypass the Outbox/Inbox guarantees this section establishes as load-bearing for correctness.

---

## 12. Failure Domains & Degradation Behavior

| Failure | Degradation Behavior |
|---|---|
| LLM Provider unavailable | `understanding:ai` queue backs up (bounded, monitored); template-path processing (`understanding:template`) continues unaffected; AI-dependent emails wait in queue for provider recovery rather than being dropped. |
| WhatsApp API unavailable | `engagement:deliver` retries with backoff; ledger/analytics/budget processing entirely unaffected (Engagement is a strict downstream leaf, per `04_domain_model.md` Section 13). |
| Gmail API rate-limited (429) | Ingestion backs off per-account; other accounts' processing unaffected (per-account isolation in queue task keys). |
| PostgreSQL primary unavailable | Full write-path outage (acceptable — this is the system of record); read replica can serve Analytics read-traffic in a degraded read-only mode if configured (detail in `19_deployment.md`). |
| Redis unavailable | Asynq cannot enqueue/dequeue — this pauses all asynchronous processing; already-committed Outbox rows remain durable in Postgres and drain once Redis recovers (Outbox durability is exactly why the pattern is anchored in Postgres, not Redis, per Section 6.1). |

This table is the architectural basis for the runbooks in `19_deployment.md` and the SLO definitions in `17_observability.md`.

---

## 13. Detailed Explanation: Why sqlc (Not a Full ORM)

- sqlc generates type-safe Go code from hand-written SQL, keeping query performance and behavior fully explicit and reviewable (no hidden N+1 queries or magic lazy-loading that a full ORM like GORM can introduce).
- This aligns with the per-module schema ownership model (Section 4): each module's sqlc queries are scoped to its own schema directory, making boundary violations (Section 5.3) easy to detect via static analysis of the generated query set.
- Combined with Goose for migrations, this gives a fully explicit, version-controlled, reviewable data-access layer appropriate for a financial system of record, where "what exact SQL runs against the ledger" should never be a mystery.

---

## 14. Component Responsibilities (Architecture-Level)

| Component | Responsibility |
|---|---|
| chi HTTP layer | Routing, request validation entry point, authentication middleware, OpenAPI-documented surface |
| Asynq workers | All asynchronous, retryable, queue-driven processing per Section 6.3's topology |
| Outbox Relay | Reliable bridge from transactional state changes to the async queue system |
| Inbox guards | Idempotent consumption at every task handler with a natural or assigned dedup key |
| PostgreSQL (per-module schema) | System of record per module; enforces INV-3 and other database-level invariants |
| Redis | Asynq backend; secondary use as a cache for read-heavy, rarely-changing lookups (e.g., known-sender registry) |
| n8n | Narrowly-scoped operational/administrative automation only — never core financial processing |
| Traefik | TLS termination, routing to API instances, rate-limiting at the edge |
| OpenTelemetry/Prometheus/Grafana | Cross-cutting observability, wired at the composition root of every module |

---

## 15. Trade-offs

| Decision | Trade-off |
|---|---|
| Modular monolith over microservices at launch | Lower immediate operational complexity, at the cost of shared-fate deployment (a bug in one module can affect the whole binary's availability) — mitigated by the strict boundary enforcement making future extraction low-risk when scale demands it. |
| Transactional Outbox (Postgres-anchored) over a dedicated event log/broker from day one | Slightly higher latency (poll-based relay) than a native CDC/broker approach, in exchange for zero additional infrastructure at launch and a straightforward upgrade path (the relay's target can become a broker later without changing producer code). |
| Per-module PostgreSQL schema (not per-module database) | Slightly less deployment isolation than fully separate databases, in exchange for simpler operational management (one Postgres cluster) at current scale, while still gaining the boundary-enforcement and future-extraction benefits. |
| Manual DI over a DI framework | Slightly more composition-root boilerplate, in exchange for full compile-time visibility of the dependency graph — valued highly for a team that must reason about correctness in a financial system. |

## 16. Advantages
- Every hotspot raised in `05_event_storming.md` has an explicit, load-bearing architectural resolution (Sections 6.5, 6.6, 6.7) rather than being left as an implementation afterthought.
- The Outbox/Inbox mechanism gives the platform genuine exactly-once *effect* guarantees without requiring exotic infrastructure, directly satisfying the SRS's hardest NFR.
- Clean Architecture + strict module boundaries make the "modular monolith today, microservices tomorrow" promise from the Vision document concretely enforceable, not aspirational.

## 17. Disadvantages
- The Outbox Relay introduces a small, additional operational component (a poller) and a modest publish-latency floor — acceptable, but it is one more thing to monitor (covered in Section 10/`17_observability.md`).
- Per-module schema isolation with no cross-schema foreign keys means some referential integrity checks that would be "free" with a FK constraint must instead be enforced in application code — a deliberate, documented trade-off (Section 5.2) rather than an oversight.

## 18. Future Improvements
- Replace the polling Outbox Relay with a Postgres logical-replication/CDC-based relay (e.g., via `pgoutput` or a tool like Debezium) if publish latency or database polling load become a measured bottleneck at scale.
- Extract the `understanding` module (AI extraction, the most independently-scalable and externally-rate-limited component) into a standalone service first, ahead of other modules, if/when load patterns justify it — the module boundary work in Section 5 is specifically designed to make this the lowest-friction first extraction candidate.
- Introduce a dedicated message broker (NATS/Kafka) as the Outbox Relay's publish target in place of direct Asynq/Redis enqueueing, if the domain-event contract needs to be consumed by systems beyond this platform's own workers (e.g., a future data warehouse/BI pipeline).

## 19. Best Practices
- Never bypass the Outbox pattern "just this once" for a synchronous-feeling shortcut — every cross-module reactive trigger must flow through it, or the exactly-once guarantee silently degrades for that one path.
- Every new Asynq task handler must be reviewed against the Inbox-pattern checklist (does it have a natural idempotency key? if not, does it correctly use `inbox_processed`?) before merge — this is a mandatory PR checklist item, formalized in `20_coding_convention.md`.
- Treat the CI-time module-boundary linter (Section 5.3) as a release-blocking check, not a warning — boundary violations must fail the pipeline, not merely be flagged.

## 20. Production Considerations
- The Outbox Relay's poll interval and batch size are production-tunable parameters that directly trade off publish latency against database load — must be load-tested and set with real traffic patterns, not launch defaults left unexamined (`23_scalability.md`).
- Redis, as both the Asynq backend and a cache, must be sized and monitored for memory pressure separately from its role as a single point of failure for all async processing (Section 12) — a Redis outage runbook is mandatory before launch (`19_deployment.md`).
- The per-module database role/permission scoping (Section 5.3) must be provisioned and tested as part of the deployment pipeline, not configured manually post-deploy, to avoid permission drift.

---

**Next Document:** `07_sequence_diagrams.md` — expands the highest-value flows from this architecture (Gmail ingestion → finalization, AI extraction fallback, budget alert firing, notification delivery, account disconnection/purge) into detailed, timed sequence diagrams showing every component interaction defined here.
