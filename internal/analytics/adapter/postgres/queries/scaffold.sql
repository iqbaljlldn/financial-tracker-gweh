-- name: GetCashFlowScaffold :many
SELECT * FROM analytics.cash_flow_view
WHERE user_id = $1;
