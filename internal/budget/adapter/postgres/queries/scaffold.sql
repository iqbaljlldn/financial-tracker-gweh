-- name: ListBudgetsScaffold :many
SELECT * FROM budget.budgets
WHERE user_id = $1;
