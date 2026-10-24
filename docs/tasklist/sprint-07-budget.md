# Sprint 7 — Budget Module

**Theme:** Build the `budget` module — threshold definitions and idempotent alert-firing logic, reacting to the ledger's `FinancialEventFinalized` stream.

**Primary documents:** `04_domain_model.md` §9; `08_database_design.md` §10; `07_sequence_diagrams.md` Diagram 6

**Entry criteria:** Sprint 6 complete — `FinancialEventFinalized` events reliably emitted.

---

## Checklist

### Budget Module — Domain Layer
- [ ] `domain/budget.go` — `Budget` aggregate (scope: global or per-category, period: monthly, `alert_thresholds`)
- [ ] `domain/savings_goal.go` — `SavingsGoal` entity
- [ ] `domain/repository.go`

### Command Handlers
- [ ] `CreateBudget`, `UpdateBudget`, `DeleteBudget`
- [ ] `CreateSavingsGoal`, `UpdateSavingsGoal`
- [ ] `EvaluateBudgetImpact` — the core reactive policy

### Idempotent Alerting **[GATE — this is the sprint's main correctness risk]**
- [ ] `budget_fired_alerts` table with `UNIQUE (budget_id, period_start, threshold)`
- [ ] `EvaluateBudgetImpact` recomputes cumulative spend on every `Expense`-type finalized event
- [ ] Threshold crossing checked against `fired_alerts` before firing — re-crossing an already-fired threshold does not re-fire
- [ ] Verify replay-safety: recomputing from the event stream never double-alerts

### Policy Wiring
- [ ] `onFinancialEventFinalized` policy (filtered to `event_type = Expense`) → `EvaluateBudgetImpact`
- [ ] `BudgetThresholdCrossed` event emitted via Outbox, consumed later by `engagement` (Sprint 9)

### Query Handlers
- [ ] `ListBudgets`, `GetBudgetProgress`
- [ ] `ListSavingsGoals` with computed `progressPercentage` from net cash flow

### API Endpoints
- [ ] `GET/POST/PATCH/DELETE /budgets`, `GET/POST/PATCH /savings-goals` (`09_api_specification.md` §6.4)

### Testing
- [ ] Unit test: threshold-crossing detection logic in isolation
- [ ] Integration test: Diagram 6 — threshold crossing fires exactly once; a second event at the same cumulative level does not re-fire
- [ ] Integration test: simulated redelivery of the same `FinancialEventFinalized` event does not double-alert

### Observability
- [ ] Metrics wired: `budget_threshold_crossed_total`, `budget_evaluation_duration_seconds` (`17_observability.md` §4.4)
- [ ] Trace span on every `EvaluateBudgetImpact` invocation, tagged with `budget.id`

### Frontend (Parallel Track)
- [ ] Budget creation/edit form: scope (global/category), period, threshold amount, alert thresholds
- [ ] Budget list/overview view with progress bars (current spend vs. threshold), color-coded at the 80%/100% marks
- [ ] Savings goal creation/edit form and progress view

---

## Exit Criteria

- [ ] A sequence of expense events crossing a defined budget threshold fires exactly one `BudgetThresholdCrossed` event
- [ ] Re-processing the same events (simulated redelivery) produces no duplicate alert
- [ ] Savings goal progress correctly reflects net cash flow from real ledger data
- [ ] Frontend: a user can create a budget, see live progress against it, and visually see when a threshold is approaching/crossed
