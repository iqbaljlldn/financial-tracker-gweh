# Sprint 6 — Financial Core: The Ledger

**Theme:** Build the `financialcore` module — the aggregate root of the entire platform. This is the highest-scrutiny sprint; invariant correctness here protects every downstream module.

**Primary documents:** `04_domain_model.md` §5; `08_database_design.md` §7; `06_architecture.md` §6; `07_sequence_diagrams.md` Diagrams 3–5

**Entry criteria:** Sprint 5 complete — both extraction paths producing symmetric `CandidateFinancialEvent`s.

---

## Checklist

### Financial Core — Domain Layer
- [ ] `domain/financial_event.go` — `FinancialEvent` aggregate, full envelope + type-specific payload per `04_domain_model.md` §5.1
- [ ] All 14 `event_type` taxonomy values implemented with their documented payload attributes **[GATE]**
- [ ] `domain/event_override.go` — `EventOverride` entity
- [ ] `domain/category_assignment.go` — `CategoryAssignment` entity
- [ ] `domain/valueobject_provenance.go` — `Provenance` value object (immutable once set)
- [ ] `domain/valueobject_confidence.go` — shared with `understanding` module's shape

### Invariant Enforcement — One Test Each, Minimum **[GATE — do not proceed without all five]**
- [ ] INV-1: cannot finalize without evaluated confidence; below-threshold → `pending_review`
- [ ] INV-2: `amount.currency` immutable; override rejects cross-currency changes with a specific error
- [ ] INV-3: `(connector_account_id, source_message_ref)` uniqueness — DB constraint, not just app-level check
- [ ] INV-4: `archived` status excluded from Analytics/Budget eligibility
- [ ] INV-5: `event_type` mutable only via `EventOverride`, never direct mutation
- [ ] Every invariant method has a doc comment citing the invariant ID per `20_coding_convention.md` §7

### Command Handlers
- [ ] `EvaluateConfidenceGate` — the gating logic itself, threshold configurable via Viper
- [ ] `ConfirmPendingReviewEvent`
- [ ] `SubmitEventOverride` — creates `EventOverride`, never mutates original extracted value
- [ ] `ArchiveFinancialEvents` (batch, used by the purge cascade later)
- [ ] Every command handler persists its state change + Outbox row in one transaction **[GATE]**

### Idempotency
- [ ] `ON CONFLICT (connector_account_id, source_message_ref) DO NOTHING` — redelivery is a successful no-op, not an error **[GATE — write the Diagram 5 test now]**
- [ ] `inbox_processed` wired for the `ConfirmPendingReviewEvent` path (guards against double-tap races)

### Categorization Integration
- [ ] `AssignCategory` policy wired to fire at **candidate-time**, not just post-finalization (H5 resolution — verify pending-review items show a suggested category) **[GATE]**

### Query Handlers
- [ ] `ListFinancialEvents` — cursor-paginated, mandatory `userID` parameter
- [ ] `GetFinancialEventByID`
- [ ] `ListEventOverrides`
- [ ] Verify: zero mutating repository calls from any query handler (code review + test assertion)

### API Endpoints
- [ ] `GET/POST /financial-events`, `PATCH /financial-events/{id}`, `POST /financial-events/{id}/confirm`, `GET /financial-events/{id}/overrides` (`09_api_specification.md` §6.2)
- [ ] Monetary fields serialized as decimal strings, never JSON floats **[GATE]**

### Testing
- [ ] Integration test: Diagram 3 full happy path (template → finalize → category → downstream Outbox events)
- [ ] Integration test: Diagram 4 pending-review → confirm → finalize
- [ ] Integration test: Diagram 5 duplicate-delivery idempotency **[GATE — this is the platform's single most important correctness test]**
- [ ] Unit test coverage ≥90% on `domain/` per `18_testing_strategy.md` §5.3 **[GATE]**

### Observability
- [ ] Metrics wired: `financialcore_events_finalized_total`, `financialcore_events_pending_review_total`, `financialcore_pending_review_sla_breach_total`, `financialcore_overrides_submitted_total`, `financialcore_dedup_conflict_total` (`17_observability.md` §4.3)
- [ ] Alert rule: pending-review SLA breach (7-day default) — don't let this accumulate silently (`17_observability.md` §8)
- [ ] Trace context captured in every `outbox_events` payload at write time (`17_observability.md` §5.2) **[GATE — the single most important tracing item in the whole series; this is the piece that keeps traces coherent across every future async hop]**

### Frontend (Parallel Track)
- [ ] Ledger/transactions list view: `GET /financial-events`, paginated (cursor-based — don't build page-number UI), filterable by date range/status/category
- [ ] Transaction detail view: full `FinancialEvent` detail incl. `typeSpecificPayload`, provenance (extraction method, confidence), and override history
- [ ] Manual override form: edit amount/merchant/category, submits via `PATCH /financial-events/{id}`, shows before/after clearly
- [ ] Pending-review list view with a web-based Confirm/Edit action (`POST /financial-events/{id}/confirm`) — a parallel path to WhatsApp confirmation (Sprint 9), useful for testing before WhatsApp templates are approved
- [ ] Decimal-string money rendering verified correct (no floating-point display bugs) across every view

---

## Exit Criteria

- [ ] An email processed through Sprints 2–5 results in a correctly finalized `FinancialEvent` in the ledger
- [ ] A simulated Asynq task redelivery produces zero duplicate ledger rows (demonstrated, not assumed)
- [ ] All five invariants have passing, named tests
- [ ] Manual override via API correctly preserves the original extracted value alongside the correction
- [ ] Frontend: a user can browse their ledger, open a transaction, correct a field, and confirm a pending-review item — entirely from the web app
