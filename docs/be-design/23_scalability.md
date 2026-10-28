# Scalability
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `03_software_requirement_specification.md`, `06_architecture.md`, `08_database_design.md`, `11_ai_blueprint.md`, `12_gmail_connector.md`, `17_observability.md`, `22_risk_analysis.md`
**Feeds Into:** `24_future_roadmap.md`

---

## 1. Purpose

This document specifies the platform's scaling strategy in concrete, actionable terms: the specific thresholds at which `financial_events` partitioning (`08_database_design.md` Section 7.1), Outbox Relay tuning (`06_architecture.md` Section 20, Risk R-T6), and module-extraction-to-microservice (`06_architecture.md` Section 18, starting with `understanding`) should be activated — plus capacity-planning guidance for Gmail/WhatsApp quota headroom (Risks R-V1, R-V2 from `22_risk_analysis.md`).

Where prior documents repeatedly said "this is tunable, revisit with production data" or "activate when a threshold is crossed," this document is where those deferred decisions get their actual trigger conditions defined — turning "we'll know it when we see it" into "here is the metric and the number that tells us."

## 2. Scope

### 2.1 In Scope
- Capacity model: from the SRS's design-point targets to concrete per-component scaling thresholds.
- `financial_events` partitioning activation threshold and procedure.
- Outbox Relay and worker horizontal-scaling thresholds.
- Module-extraction decision criteria and sequencing (starting with `understanding`).
- Gmail API and WhatsApp Business API quota/tier capacity planning.
- Database read-replica and connection-pooling scaling guidance.

### 2.2 Out of Scope
- Actual infrastructure-as-code autoscaling configuration (implementation deliverable).
- Cost-optimization modeling beyond what's needed to size capacity correctly (a FinOps concern, referenced but not exhaustively modeled here).

---

## 3. Architecture: Scaling Dimensions Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Dimension 1: Stateless compute scaling                          │
│  (API instances, worker instances — horizontal, elastic)          │
├─────────────────────────────────────────────────────────────┤
│  Dimension 2: Stateful data scaling                               │
│  (PostgreSQL partitioning, read replicas, connection pooling)      │
├─────────────────────────────────────────────────────────────┤
│  Dimension 3: External quota/rate-limit scaling                   │
│  (Gmail API quota, WhatsApp messaging tier, LLM provider rate limit)│
├─────────────────────────────────────────────────────────────┤
│  Dimension 4: Architectural scaling                               │
│  (module extraction to standalone services, when horizontal        │
│   compute scaling alone is no longer sufficient for a specific     │
│   module's independent load profile)                               │
└─────────────────────────────────────────────────────────────┘
```

Each dimension is scaled **independently and on its own trigger**, not as a single "scale the whole platform" operation — this is a direct consequence of the modular-monolith, per-module-schema, dual-mode-binary design established in `06_architecture.md`, which exists specifically to make this kind of independent-dimension scaling possible without a full system redesign.

---

## 4. Dimension 1: Stateless Compute Scaling

### 4.1 API Instances

- **Trigger metric:** `http_request_duration_seconds` P95 approaching the SRS Section 6.1 target ceiling, or CPU/memory utilization crossing a standard 70% sustained threshold.
- **Action:** Horizontal scale-out of API instances (stateless, per `06_architecture.md` Section 11's dual-mode binary design — adding an instance requires no state migration).
- **No upper-bound architectural constraint:** API instances have no inherent scaling ceiling in this design; this dimension is the cheapest and most straightforward to scale, and should be the **first** lever reached for before considering any more invasive change (partitioning, module extraction).

### 4.2 Worker Instances (Per-Queue Scaling)

- **Trigger metric:** Per-queue depth and oldest-task-age (`17_observability.md` Section 4's queue metrics), evaluated **per queue**, not as an aggregate worker-pool metric — since `06_architecture.md` Section 6.3's topology deliberately gives different queues different concurrency/rate-limit characteristics, a generic "scale workers when busy" rule would incorrectly scale worker count for a queue that's actually backpressure-limited by an external rate limit (Section 6, below) rather than compute-limited.
- **Action:** Scale worker instance count for compute-bound queues (`understanding:template`, `financialcore:finalize`, `analytics:project`) in response to queue depth. For externally-rate-limited queues (`understanding:ai`, `engagement:deliver`), **adding more worker instances does not help** once the external rate limit is saturated — the correct response is Dimension 3 (quota/tier capacity planning), not Dimension 1 scaling. This distinction is a mandatory on-call judgment call, explicitly documented here to prevent the common mistake of "just add more workers" when the actual bottleneck is external.

---

## 5. Dimension 2: Stateful Data Scaling

### 5.1 `financial_events` Partitioning Activation

Per `08_database_design.md` Section 7.1's "designed for, not yet activated" partitioning:

- **Activation threshold:** Partitioning (monthly range partitions on `occurred_at`) is activated when the table crosses **either** (a) 50 million rows, **or** (b) a sustained query-latency regression on the primary access-pattern indexes (`idx_financial_events_user_occurred`, Section 7.1 of that document) exceeding the SRS Section 6.1 analytics-query latency target — whichever threshold is reached first. These are **starting guidance values**, explicitly flagged (per `08_database_design.md` Section 19) for revision against real production volume/performance data, not fixed constants to apply blindly.
- **Procedure:** Activated via a Goose migration introducing the partition structure and a backfill process (per `19_deployment.md` Section 6's backward-compatible migration discipline — the partitioning migration must be designed so the application layer's query behavior is unaffected, since sqlc-generated queries against a partitioned table with correctly-configured constraint exclusion behave identically from the application's perspective).
- **Why monthly, not another interval:** Monthly partitions align with the platform's dominant query pattern (`cash_flow_view`, `trend_view` per `08_database_design.md` Section 11, both naturally month-bucketed), maximizing partition-pruning effectiveness for the queries that matter most, while keeping partition count manageable (12/year, not 365/year as daily partitioning would produce).

### 5.2 Read Replica Introduction

- **Trigger:** Sustained primary-database read load (particularly from Analytics query traffic, per `06_architecture.md` Section 7's CQRS read-side) competing meaningfully with write-path latency (Financial Core's finalization path, per the SRS's stricter latency target there).
- **Action:** Introduce a read replica specifically for Analytics module query traffic (per `06_architecture.md` Section 11's noted "read replica can serve Analytics read-traffic in a degraded read-only mode" capability, promoted here from a failure-mode capability to a standing, deliberate load-distribution strategy once the trigger condition is met) — Analytics is the natural first consumer of read-replica capacity since its CQRS read-model design already tolerates eventual consistency (per `06_architecture.md` Section 7's rebuildability requirement), making replica lag a non-issue for that specific module in a way it would not be for a hypothetical direct read against Financial Core's write-path tables.

### 5.3 Connection Pooling

- A connection pooler (e.g., PgBouncer or equivalent, positioned between application instances and PostgreSQL) is introduced once the combined API+worker instance count's direct connection count approaches PostgreSQL's configured `max_connections` ceiling — a standard, well-understood scaling lever, flagged here specifically because the platform's per-module schema design (`06_architecture.md` Section 4) and per-module database roles (`06_architecture.md` Section 5.3) mean connection pooling configuration must be done **per-role**, not as a single generic pool, to preserve the schema-permission isolation that's load-bearing for the module-boundary enforcement design.

---

## 6. Dimension 3: External Quota/Rate-Limit Capacity Planning

### 6.1 Gmail API Quota (Risk R-V1)

- **Capacity model:** Quota consumption scales roughly linearly with (a) linked-mailbox count × (b) the configured poll-fallback interval's inverse (`12_gmail_connector.md` Section 5.3 — a shorter interval consumes proportionally more quota) × (c) per-account message volume.
- **Planning trigger:** When projected quota consumption (tracked via `ingestion_gmail_quota_units_consumed_total`, per `17_observability.md` Section 4.1) crosses 70% of the currently-granted project quota, a Google Cloud quota-increase request must be filed — **not** at 90%+ as a reactive measure, since Google's quota-increase process has **non-trivial, variable lead time** (`12_gmail_connector.md` Section 17's explicit note), making 70% the point where "file the request now" still leaves adequate margin before actually hitting the ceiling.
- **Available levers before requesting a quota increase:** Widening the poll-fallback interval (`12_gmail_connector.md` Section 5.3's explicitly-tunable default) is the first, cheapest lever — trading slightly higher worst-case latency-during-push-outage for immediate quota relief — before escalating to a formal quota-increase request, which is the second lever once interval-widening alone is insufficient.

### 6.2 WhatsApp Business API Messaging Tier (Risk R-V2)

- **Capacity model:** The messaging tier (`15_notification_system.md` Section 9) grows based on account quality/usage history — it is **not** a capacity dimension the platform can request to increase on demand the way Gmail quota can; it is earned through sustained good standing (low block rate, high engagement).
- **Planning trigger:** When projected daily proactive-message volume (driven by user-base growth × notification frequency, a straightforward multiplication given the fixed `notification_type` catalog from `15_notification_system.md` Section 4.2) approaches the current tier's ceiling, the **only** scalable responses are: (a) grow the user base more slowly than the tier naturally expands (not typically acceptable), (b) actively manage the quality-rating signals (block rate, Section 9 of that document) to accelerate tier growth, or (c) reduce proactive-message frequency per user (e.g., consolidating daily summaries into a less frequent cadence) — this is fundamentally a **product-lever**, not a pure engineering-scaling lever, and must be planned jointly with product leadership well ahead of the ceiling being reached, per `15_notification_system.md` Section 17's cross-functional note.

### 6.3 LLM Provider Rate Limits

- **Capacity model:** Bounded by the `understanding:ai` queue's configured concurrency ceiling (`06_architecture.md` Section 6.5), itself bounded by the provider's rate limit.
- **Planning trigger:** Since this platform's design philosophy (`01_product_vision.md`'s "Deterministic Before Probabilistic") deliberately keeps AI-extraction volume to a minority of total traffic (via the known-sender template library, per `02_product_requirement_document.md` Section 6.1), this dimension should scale more slowly than raw user-base growth — a sustained, disproportionate rise in AI-extraction volume *relative to* user-base growth is itself a signal to investigate (is the template library coverage falling behind, per Risk R-P2 from `22_risk_analysis.md`, rather than a signal to simply request a higher provider rate limit).

---

## 7. Dimension 4: Architectural Scaling — Module Extraction

### 7.1 Extraction Decision Criteria

A module is a candidate for extraction into a standalone service (per `06_architecture.md` Section 18's noted possibility) when **any** of the following hold, evaluated against real production telemetry:

1. **Independent load profile divergence:** The module's resource consumption pattern (CPU/memory/network shape) differs so significantly from the rest of the monolith that co-deployed horizontal scaling (Section 4) forces over-provisioning of the other modules just to give this one module enough headroom.
2. **Independent deployment cadence need:** The module requires materially more frequent releases than the rest of the system (e.g., rapid AI prompt/model iteration) to the point where the shared-binary release cadence (`19_deployment.md` Section 3's promotion flow) becomes a bottleneck specifically for that module's iteration speed.
3. **Independent team ownership:** A dedicated team forms around the module's domain (e.g., a specialized ML/AI team owning `understanding`) whose workflow would benefit from an independently deployable, independently on-call service boundary.

### 7.2 Why `understanding` Is the Designated First Candidate

Per `06_architecture.md` Section 18 and `10_backend_blueprint.md` Section 18, `understanding` is explicitly pre-identified as the most likely first extraction candidate, because it uniquely satisfies **all three** Section 7.1 criteria simultaneously:
- Its load profile (bursty, externally-rate-limited LLM calls) is genuinely different from every other module's (mostly database-bound) profile.
- AI/prompt iteration (per `11_ai_blueprint.md` Section 4.3's regression-gated promotion process) is plausibly a higher-frequency change category than, say, `budget`'s relatively stable business rules.
- It is the module most likely to attract specialized ownership as the platform's AI capabilities mature toward Phase 3's advisory intelligence (`01_product_vision.md` Stage 4).

### 7.3 Extraction Procedure (High-Level)

Because `10_backend_blueprint.md` Section 18 explicitly designed the adapter-layer separation anticipating this: extraction primarily involves (a) moving the module's directory to its own repository/deployable unit, (b) swapping its Outbox-publish and Inbox-consume bindings from in-process Asynq/Redis to a network-based equivalent (the domain event *contracts* are unchanged — only the *transport* changes, per `06_architecture.md` Section 18's stated design intent), and (c) establishing a network-callable version of any cross-module interface it previously implemented in-process (e.g., if `understanding` ever published an interface another module consumed synchronously — current design has no such case, per `10_backend_blueprint.md` Section 6's table, which is itself a deliberate simplification that makes this specific extraction easier). Domain and Application layers require **minimal to no change** — this is the direct payoff of the Clean Architecture dependency discipline maintained throughout the series.

---

## 8. Detailed Explanation: Why Module Extraction Is a Last-Resort Lever, Not a Default Scaling Response

Given how cleanly this architecture supports extraction (Section 7.3), it might seem tempting to extract modules proactively, ahead of any real pressure, simply because the design makes it "easy." This document explicitly discourages that temptation:

1. **Dimension 1 (stateless compute scaling) and Dimension 2 (data scaling) are categorically cheaper, faster, and lower-risk than Dimension 4** — scaling an API instance count or activating a planned partitioning scheme are routine, well-understood operations; extracting a module introduces genuinely new operational surface (a new deployable, new network boundary, new failure mode — a network partition between the extracted service and the rest of the platform did not exist as a risk before extraction and does after).
2. **`06_architecture.md` Section 3.1's original justification for the modular monolith** — "a modular monolith with enforced boundaries gets ~80% of microservices' maintainability benefit... at a fraction of the operational cost" — remains true until a *specific, evidenced* pressure (Section 7.1's criteria) makes the remaining 20% worth the operational cost increase. Extracting prematurely, before that evidence exists, pays the operational cost without yet realizing a commensurate benefit — the exact inverse of a sound scaling decision.
3. **This is the same "don't over-engineer ahead of evidence" discipline already applied elsewhere in the series** — `08_database_design.md` designed for partitioning without activating it prematurely; `17_observability.md` deferred burn-rate alerting until sufficient production data exists; this document's Section 7 extends that same discipline to the single most consequential, hardest-to-reverse scaling lever available.

---

## 9. Component Responsibilities

| Component | Responsibility |
|---|---|
| API/Worker horizontal scaling (Dimension 1) | First-line, cheapest, always-available scaling response |
| Partitioning/read-replica/pooling (Dimension 2) | Data-layer scaling, activated on evidenced thresholds, not preemptively |
| Quota/tier capacity planning process (Dimension 3) | Proactive, lead-time-aware management of external, non-elastic ceilings |
| Module extraction decision process (Dimension 4) | Last-resort, evidence-gated architectural scaling, reserved for genuine divergent pressure |

---

## 10. Trade-offs

| Decision | Trade-off |
|---|---|
| Per-queue (not aggregate) worker-scaling triggers | More nuanced, judgment-requiring on-call decision-making than a single generic autoscaling rule, in exchange for correctly distinguishing compute-bound from externally-rate-limited bottlenecks, avoiding wasted scale-out effort on the latter. |
| 70%-quota-consumption filing trigger for Gmail (not 90%+) | Requests a quota increase earlier than strictly necessary by the numbers alone, in exchange for absorbing Google's variable approval lead time safely, consistent with `12_gmail_connector.md`'s already-stated caution on this exact point. |
| Module extraction reserved as a last-resort, evidence-gated lever | Forgoes the "it's easy, let's just do it" appeal of early extraction, in exchange for never paying Dimension 4's operational cost before its benefit is actually needed. |

## 11. Advantages
- Every previously-deferred "tune this with real data later" note across the prior twenty-two documents now has an explicit trigger condition and owning process here, closing the loop on the series' many forward references to exactly this document.
- The four-dimension framing makes explicit that "scaling" is not one decision but several independent ones, each with its own cheapest-first lever — directly preventing the common failure mode of reaching for an expensive, invasive scaling response (module extraction) when a cheap one (horizontal compute scaling) would have sufficed.
- The WhatsApp tier-growth framing (Section 6.2) correctly identifies that not every capacity dimension is purely an engineering lever — some require explicit, early cross-functional planning, a nuance a purely technical scalability document could easily have missed.

## 12. Disadvantages
- Several thresholds in this document (the 50-million-row partitioning trigger, the 70% Gmail quota trigger) are, by this document's own admission, starting estimates requiring validation against real production data — meaning this document itself will need a revision pass once meaningful production history accumulates, rather than being a "set once" artifact.
- The module-extraction decision criteria (Section 7.1), while concrete, still require human judgment to apply (there's no single metric threshold that mechanically triggers extraction the way partitioning or worker-scaling thresholds do) — an inherent property of a decision this architecturally significant, but worth noting as a real difference in rigor from this document's other sections.

## 13. Future Improvements
- Once production telemetry exists, replace Section 5.1's partitioning threshold and Section 6.1's Gmail-quota-planning percentages with empirically-validated figures, and record the validation basis explicitly (mirroring `22_risk_analysis.md` Section 16's directive to recalibrate likelihood estimates with observed data).
- Build a lightweight internal capacity-planning dashboard (extending `17_observability.md` Section 7's "Cost & Capacity" dashboard) that directly visualizes current consumption against each dimension's documented trigger threshold, rather than requiring an engineer to manually compare a raw metric against this document's prose thresholds.
- Revisit whether a second module (beyond `understanding`) becomes a credible extraction candidate as the platform's Phase 3 advisory-intelligence features (`01_product_vision.md` Stage 4) mature — `financialcore` or a new "Advisory" bounded context are plausible future candidates worth re-evaluating against Section 7.1's criteria at that point.

## 14. Best Practices
- Any engineer proposing a scaling action must identify which of this document's four dimensions it belongs to and cite the specific trigger condition being responded to — "it seemed slow so I added more workers" is not an acceptable scaling rationale under this document's discipline; "queue X's depth crossed threshold Y per Section 4.2" is.
- Module extraction (Dimension 4) must never be proposed or undertaken without an explicit review against Section 7.1's three criteria, documented in the proposal — extraction is irreversible-in-practice enough (a genuine undo requires re-merging a service back into the monolith, a nontrivial reverse migration) that it warrants this explicit, written justification gate.
- This document's thresholds should be reviewed at the same quarterly cadence established in `22_risk_analysis.md` Section 10, treating capacity-planning thresholds with the same "living document, not a one-time artifact" discipline applied to that risk register.

## 15. Production Considerations
- The Gmail-quota and WhatsApp-tier capacity dashboards (feeding Sections 6.1/6.2's trigger monitoring) must be operational **before** the user-base growth that would make hitting either ceiling plausible — building this visibility reactively, after a near-miss, defeats the entire purpose of proactive capacity planning.
- Partitioning activation (Section 5.1), when it occurs, must be rehearsed in staging against a production-volume-representative dataset before being applied to production — a migration of this structural significance against the platform's single most important table warrants the same deliberate, observed-rehearsal discipline `18_testing_strategy.md` Section 19 mandated for the GDPR purge cascade.
- The module-extraction procedure (Section 7.3), even though not currently triggered, should have its network-transport-swap mechanics prototyped or at least detailed in a design spike well before Section 7.1's criteria are actually met — so that when the decision to extract is made, the "how" is already de-risked and the team isn't designing the extraction mechanics under the time pressure of an already-evidenced scaling problem.

---

**Next Document:** `24_future_roadmap.md` — the final document in the series: consolidates the Vision's Stage 1–4 trajectory, the PRD's MVP/Phase 2/Phase 3 scoping, and every "Future Improvement" section scattered across all twenty-three prior documents into a single, prioritized, forward-looking roadmap.
