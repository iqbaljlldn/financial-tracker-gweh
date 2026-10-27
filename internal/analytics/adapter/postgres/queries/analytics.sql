-- Cash Flow Queries
-- name: UpsertCashFlow :exec
INSERT INTO analytics.cash_flow_view (
    user_id, period_bucket, granularity, total_income_value, total_expense_value, currency, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (user_id, period_bucket, granularity, currency)
DO UPDATE SET
    total_income_value = analytics.cash_flow_view.total_income_value + EXCLUDED.total_income_value,
    total_expense_value = analytics.cash_flow_view.total_expense_value + EXCLUDED.total_expense_value,
    updated_at = EXCLUDED.updated_at;

-- name: GetCashFlow :many
SELECT user_id, period_bucket, granularity, total_income_value, total_expense_value, currency, updated_at
FROM analytics.cash_flow_view
WHERE user_id = $1
  AND granularity = $2
  AND ($3::date IS NULL OR period_bucket >= $3)
  AND ($4::date IS NULL OR period_bucket <= $4)
  AND (NULLIF($5::text, '') IS NULL OR currency = $5)
ORDER BY period_bucket ASC, currency ASC;

-- Category Breakdown Queries
-- name: UpsertCategoryBreakdown :exec
INSERT INTO analytics.category_breakdown_view (
    user_id, period_bucket, category, total_value, currency, percentage_share, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (user_id, period_bucket, category, currency)
DO UPDATE SET
    total_value = analytics.category_breakdown_view.total_value + EXCLUDED.total_value,
    percentage_share = EXCLUDED.percentage_share,
    updated_at = EXCLUDED.updated_at;

-- name: RecalculateCategoryPercentageShares :exec
UPDATE analytics.category_breakdown_view
SET percentage_share = ROUND(COALESCE((category_breakdown_view.total_value / totals.total_sum * 100), 0)::numeric, 2),
    updated_at = $4
FROM (
    SELECT NULLIF(SUM(total_value), 0) AS total_sum
    FROM analytics.category_breakdown_view
    WHERE user_id = $1 AND period_bucket = $2 AND currency = $3
) AS totals
WHERE category_breakdown_view.user_id = $1
  AND category_breakdown_view.period_bucket = $2
  AND category_breakdown_view.currency = $3;

-- name: GetCategoryBreakdown :many
SELECT user_id, period_bucket, category, total_value, currency, percentage_share, updated_at
FROM analytics.category_breakdown_view
WHERE user_id = $1
  AND ($2::date IS NULL OR period_bucket >= $2)
  AND ($3::date IS NULL OR period_bucket <= $3)
  AND (NULLIF($4::text, '') IS NULL OR currency = $4)
ORDER BY total_value DESC;

-- Merchant Ranking Queries
-- name: UpsertMerchantRanking :exec
INSERT INTO analytics.merchant_ranking_view (
    user_id, period_bucket, merchant_normalized_name, total_value, transaction_count, currency, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (user_id, period_bucket, merchant_normalized_name, currency)
DO UPDATE SET
    total_value = analytics.merchant_ranking_view.total_value + EXCLUDED.total_value,
    transaction_count = analytics.merchant_ranking_view.transaction_count + EXCLUDED.transaction_count,
    updated_at = EXCLUDED.updated_at;

-- name: GetMerchantRanking :many
SELECT user_id, period_bucket, merchant_normalized_name, total_value, transaction_count, currency, updated_at
FROM analytics.merchant_ranking_view
WHERE user_id = $1
  AND ($2::date IS NULL OR period_bucket >= $2)
  AND ($3::date IS NULL OR period_bucket <= $3)
  AND (NULLIF($4::text, '') IS NULL OR currency = $4)
ORDER BY total_value DESC
LIMIT $5;

-- Trend Queries
-- name: UpsertTrend :exec
INSERT INTO analytics.trend_view (
    user_id, period_bucket, granularity, net_flow_value, currency, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (user_id, period_bucket, granularity, currency)
DO UPDATE SET
    net_flow_value = analytics.trend_view.net_flow_value + EXCLUDED.net_flow_value,
    updated_at = EXCLUDED.updated_at;

-- name: GetTrend :many
SELECT user_id, period_bucket, granularity, net_flow_value, currency, updated_at
FROM analytics.trend_view
WHERE user_id = $1
  AND granularity = $2
  AND ($3::date IS NULL OR period_bucket >= $3)
  AND ($4::date IS NULL OR period_bucket <= $4)
  AND (NULLIF($5::text, '') IS NULL OR currency = $5)
ORDER BY period_bucket ASC, currency ASC;

-- Projection Checkpoint Queries
-- name: UpsertProjectionCheckpoint :exec
INSERT INTO analytics.projection_checkpoint (
    source_module, last_processed_outbox_event_id, checkpoint_at
) VALUES (
    $1, $2, $3
)
ON CONFLICT (source_module)
DO UPDATE SET
    last_processed_outbox_event_id = EXCLUDED.last_processed_outbox_event_id,
    checkpoint_at = EXCLUDED.checkpoint_at;

-- name: GetProjectionCheckpoint :one
SELECT source_module, last_processed_outbox_event_id, checkpoint_at
FROM analytics.projection_checkpoint
WHERE source_module = $1;

-- Projected Events Queries
-- name: SaveProjectedEvent :exec
INSERT INTO analytics.projected_events (
    event_id, user_id, event_type, amount_value, currency, occurred_at,
    merchant_normalized_name, category, status, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (event_id)
DO UPDATE SET
    event_type = EXCLUDED.event_type,
    amount_value = EXCLUDED.amount_value,
    currency = EXCLUDED.currency,
    occurred_at = EXCLUDED.occurred_at,
    merchant_normalized_name = EXCLUDED.merchant_normalized_name,
    category = EXCLUDED.category,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

-- name: GetProjectedEvent :one
SELECT event_id, user_id, event_type, amount_value, currency, occurred_at,
       merchant_normalized_name, category, status, updated_at
FROM analytics.projected_events
WHERE event_id = $1;

-- name: WipeAnalyticsData :exec
TRUNCATE analytics.cash_flow_view,
         analytics.category_breakdown_view,
         analytics.merchant_ranking_view,
         analytics.trend_view,
         analytics.projected_events,
         analytics.projection_checkpoint;
