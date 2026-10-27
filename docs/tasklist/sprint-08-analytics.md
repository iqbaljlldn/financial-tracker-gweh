# Sprint 8 — Analytics Module (CQRS Read Side)

**Theme:** Build the `analytics` module — the platform's sole read-optimized, rebuildable projection layer over the ledger. No write authority over `financialcore` anywhere in this module.

**Primary documents:** `06_architecture.md` §7; `08_database_design.md` §11, §14; `04_domain_model.md` §10

**Entry criteria:** Sprint 6 complete (ledger emitting `FinancialEventFinalized`/`Overridden`/`Archived`/`CategoryAssignmentChanged`); Sprint 7 complete (nice-to-have, not hard blocker).

---

## Checklist

### Analytics Module — Structure
- [x] **No `application/command/` package** — this module is query + projection-policy only **[GATE — verify this structurally, per `10_backend_blueprint.md` §4.2]**
- [x] `domain/` — thin, mostly just the read-model shapes (not full aggregates, since this is a CQRS read side)

### Read-Model Tables
- [x] `cash_flow_view`, `category_breakdown_view`, `merchant_ranking_view`, `trend_view` — all currency-partitioned (composite PK includes `currency`) **[GATE — no default currency blending]** (`08_database_design.md` §11, §14)
- [x] `projection_checkpoint` — tracks last-processed Outbox event per source module

### Projection Policy
- [x] `ProjectAnalyticsReadModels` policy handler, subscribed to all four Financial Core event types
- [x] Upsert logic per view, correctly handling currency partitioning
- [x] Checkpoint updated transactionally with the projection write (crash-recovery resumability)
- [x] Verify full rebuildability: wipe the analytics schema, replay from Outbox history, confirm identical resulting state **[GATE — required by SRS §6.3]**

### Query Handlers
- [x] `GetCashFlow`, `GetCategoryBreakdown`, `GetMerchantRanking`, `GetTrend` — read exclusively from this module's own schema, zero `financialcore` access **[GATE]**

### API Endpoints
- [x] `GET /analytics/cash-flow`, `/category-breakdown`, `/merchant-ranking`, `/trend` (`09_api_specification.md` §6.3)
- [x] `projectionFreshness` field included in every response, sourced from `projection_checkpoint`

### Observability
- [x] `analytics_projection_lag_seconds` histogram, per view (`17_observability.md` §4.5) — this is the H6 resolution, don't skip it
- [x] `analytics_projection_checkpoint_age_seconds` gauge
- [x] `analytics_query_duration_seconds` histogram per endpoint
- [x] Grafana "Analytics Freshness" dashboard stood up (`17_observability.md` §7)

### Frontend (Parallel Track)
- [x] Main dashboard view: cash flow chart (income vs. expense over time), using a real charting library
- [x] Category breakdown view: pie/bar chart + percentage-share table
- [x] Merchant ranking view: sortable table, top merchants by spend/frequency
- [x] Trend view: daily/monthly net-flow line chart with period comparison (this month vs. last month)
- [x] Multi-currency handling in the UI: if a user has events in >1 currency, show separate totals per currency — never silently sum them (matches the backend's currency-partitioned design, `08_database_design.md` §14)
- [x] Surface `projectionFreshness` subtly in the UI (e.g., "updated 2 min ago") so staleness is never a silent surprise

### Testing
- [x] Integration test: finalize events of known amounts/categories/dates, verify projected view matches hand-computed expected aggregates
- [x] Integration test: full rebuild-from-replay produces identical state to incremental projection
- [x] Integration test: multi-currency events produce separate, correctly-partitioned rows (never blended)

---

## Exit Criteria

- [x] Dashboard-ready aggregates are queryable and match manually-verified expected values against seeded test data
- [x] A full read-model wipe-and-rebuild from the Outbox event history produces identical results
- [x] `projectionFreshness` is visibly non-stale under normal operation
- [x] Frontend: a user can open the dashboard and see correct, correctly-charted cash flow, category breakdown, merchant ranking, and trend data from their own real (test) ledger
