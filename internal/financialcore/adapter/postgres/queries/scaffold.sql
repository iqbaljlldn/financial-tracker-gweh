-- name: ListFinancialEventsScaffold :many
SELECT * FROM financialcore.financial_events
WHERE user_id = $1
ORDER BY occurred_at DESC
LIMIT $2;
