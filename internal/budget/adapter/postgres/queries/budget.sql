-- name: CreateBudget :one
INSERT INTO budget.budgets (
    id, user_id, scope, period_type, threshold_amount_value, threshold_amount_currency, alert_thresholds, active, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: UpdateBudget :one
UPDATE budget.budgets
SET scope = $2,
    period_type = $3,
    threshold_amount_value = $4,
    threshold_amount_currency = $5,
    alert_thresholds = $6,
    active = $7,
    updated_at = $8
WHERE id = $1 AND user_id = $9
RETURNING *;

-- name: GetBudgetByID :one
SELECT * FROM budget.budgets
WHERE id = $1 AND user_id = $2;

-- name: ListBudgetsByUserID :many
SELECT * FROM budget.budgets
WHERE user_id = $1
  AND (active = true OR @include_inactive::boolean = true)
ORDER BY created_at DESC;

-- name: ListMatchingBudgets :many
SELECT * FROM budget.budgets
WHERE user_id = $1
  AND active = true
  AND (scope = 'global' OR scope = @category_scope::text);

-- name: DeleteBudget :execrows
DELETE FROM budget.budgets
WHERE id = $1 AND user_id = $2;

-- name: RecordFiredAlert :execrows
INSERT INTO budget.budget_fired_alerts (
    id, budget_id, period_start, threshold, fired_at
) VALUES (
    $1, $2, $3, $4, $5
) ON CONFLICT (budget_id, period_start, threshold) DO NOTHING;

-- name: ListFiredAlerts :many
SELECT threshold
FROM budget.budget_fired_alerts
WHERE budget_id = $1 AND period_start = $2;

-- name: CreateSavingsGoal :one
INSERT INTO budget.savings_goals (
    id, user_id, target_amount_value, target_amount_currency, target_date, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: UpdateSavingsGoal :one
UPDATE budget.savings_goals
SET target_amount_value = $2,
    target_amount_currency = $3,
    target_date = $4,
    updated_at = $5
WHERE id = $1 AND user_id = $6
RETURNING *;

-- name: GetSavingsGoalByID :one
SELECT * FROM budget.savings_goals
WHERE id = $1 AND user_id = $2;

-- name: ListSavingsGoalsByUserID :many
SELECT * FROM budget.savings_goals
WHERE user_id = $1
ORDER BY target_date ASC;

-- name: DeleteSavingsGoal :execrows
DELETE FROM budget.savings_goals
WHERE id = $1 AND user_id = $2;

-- name: RecordOutboxEvent :exec
INSERT INTO budget.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at, relayed
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, false
);

-- name: ListUnrelayedOutboxEvents :many
SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at
FROM budget.outbox_events
WHERE relayed = false
ORDER BY created_at ASC
LIMIT $1;

-- name: MarkOutboxEventsRelayed :exec
UPDATE budget.outbox_events
SET relayed = true, relayed_at = now()
WHERE id = ANY(@ids::uuid[]);
