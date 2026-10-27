-- Financial Core ledger queries (08_database_design.md §7). All queries are schema-isolated to financialcore.

-- name: InsertFinancialEventIfNew :one
-- INV-3: idempotent insert; a redelivery hits uq_financial_events_provenance and returns no row.
INSERT INTO financialcore.financial_events (
    id, user_id, event_type, amount_value, amount_currency, occurred_at,
    merchant_raw_name, merchant_normalized_name, payment_method_type, payment_method_hint,
    status, confidence_aggregate, confidence_method, type_specific_payload,
    provenance_connector_type, provenance_connector_account_id, provenance_source_message_ref,
    provenance_extraction_method, provenance_template_version, provenance_prompt_version,
    provenance_model_identifier, provenance_extracted_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17,
    $18, $19, $20,
    $21, $22, $23, $24
)
ON CONFLICT ON CONSTRAINT uq_financial_events_provenance DO NOTHING
RETURNING id;

-- name: GetFinancialEvent :one
SELECT
    id, user_id, event_type, amount_value::text AS amount_value, amount_currency, occurred_at,
    merchant_raw_name, merchant_normalized_name, payment_method_type, payment_method_hint,
    status, confidence_aggregate::float8 AS confidence_aggregate, confidence_method, type_specific_payload,
    provenance_connector_type, provenance_connector_account_id, provenance_source_message_ref,
    provenance_extraction_method, provenance_template_version, provenance_prompt_version,
    provenance_model_identifier, provenance_extracted_at, created_at, updated_at
FROM financialcore.financial_events
WHERE id = $1 AND user_id = $2;

-- name: GetFinancialEventForUpdate :one
SELECT
    id, user_id, event_type, amount_value::text AS amount_value, amount_currency, occurred_at,
    merchant_raw_name, merchant_normalized_name, payment_method_type, payment_method_hint,
    status, confidence_aggregate::float8 AS confidence_aggregate, confidence_method, type_specific_payload,
    provenance_connector_type, provenance_connector_account_id, provenance_source_message_ref,
    provenance_extraction_method, provenance_template_version, provenance_prompt_version,
    provenance_model_identifier, provenance_extracted_at, created_at, updated_at
FROM financialcore.financial_events
WHERE id = $1 AND user_id = $2
FOR UPDATE;

-- name: ListFinancialEvents :many
-- Keyset pagination on (occurred_at DESC, id DESC). user_id is mandatory. eventType and merchant filters
-- match the EFFECTIVE value (latest override, else original); category matches the current assignment.
SELECT
    e.id, e.user_id, e.event_type, e.amount_value::text AS amount_value, e.amount_currency, e.occurred_at,
    e.merchant_raw_name, e.merchant_normalized_name, e.payment_method_type, e.payment_method_hint,
    e.status, e.confidence_aggregate::float8 AS confidence_aggregate, e.confidence_method, e.type_specific_payload,
    e.provenance_connector_type, e.provenance_connector_account_id, e.provenance_source_message_ref,
    e.provenance_extraction_method, e.provenance_template_version, e.provenance_prompt_version,
    e.provenance_model_identifier, e.provenance_extracted_at, e.created_at, e.updated_at
FROM financialcore.financial_events e
WHERE e.user_id = @user_id::uuid
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR e.occurred_at >= sqlc.narg(from_ts)::timestamptz)
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR e.occurred_at < sqlc.narg(to_ts)::timestamptz)
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status)::text)
  AND (sqlc.narg(event_type)::text IS NULL OR COALESCE(
        (SELECT o.new_value #>> '{}' FROM financialcore.event_overrides o
          WHERE o.financial_event_id = e.id AND o.field_name = 'eventType'
          ORDER BY o.corrected_at DESC, o.id DESC LIMIT 1),
        e.event_type) = sqlc.narg(event_type)::text)
  AND (sqlc.narg(merchant)::text IS NULL OR lower(COALESCE(
        (SELECT o.new_value #>> '{}' FROM financialcore.event_overrides o
          WHERE o.financial_event_id = e.id AND o.field_name = 'merchant'
          ORDER BY o.corrected_at DESC, o.id DESC LIMIT 1),
        e.merchant_normalized_name, e.merchant_raw_name, '')) = lower(sqlc.narg(merchant)::text))
  AND (sqlc.narg(category)::text IS NULL OR EXISTS (
        SELECT 1 FROM financialcore.category_assignments c
         WHERE c.financial_event_id = e.id AND c.is_current = true AND c.category = sqlc.narg(category)::text))
  AND (sqlc.narg(cursor_ts)::timestamptz IS NULL
       OR (e.occurred_at, e.id) < (sqlc.narg(cursor_ts)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY e.occurred_at DESC, e.id DESC
LIMIT @page_limit::int;

-- name: UpdateFinancialEventStatus :exec
UPDATE financialcore.financial_events
SET status = $2, updated_at = $3
WHERE id = $1;

-- name: ArchiveFinancialEventsByIDs :many
UPDATE financialcore.financial_events
SET status = 'archived', updated_at = @now::timestamptz
WHERE user_id = @user_id::uuid
  AND id = ANY(@ids::uuid[])
  AND status <> 'archived'
RETURNING id;

-- name: ArchiveFinancialEventsByConnectorAccount :many
UPDATE financialcore.financial_events
SET status = 'archived', updated_at = @now::timestamptz
WHERE user_id = @user_id::uuid
  AND provenance_connector_account_id = @connector_account_id::uuid
  AND status <> 'archived'
RETURNING id;

-- name: PermanentlyEraseArchivedEvents :execrows
DELETE FROM financialcore.financial_events
WHERE status = 'archived'
  AND updated_at < @older_than::timestamptz;

-- name: InsertEventOverride :exec
INSERT INTO financialcore.event_overrides (
    id, financial_event_id, field_name, previous_value, new_value, actor_user_id, corrected_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListEventOverridesForEvent :many
SELECT id, financial_event_id, field_name, previous_value, new_value, actor_user_id, corrected_at
FROM financialcore.event_overrides
WHERE financial_event_id = $1
ORDER BY corrected_at ASC, id ASC;

-- name: ListEventOverridesForEvents :many
SELECT id, financial_event_id, field_name, previous_value, new_value, actor_user_id, corrected_at
FROM financialcore.event_overrides
WHERE financial_event_id = ANY(@event_ids::uuid[])
ORDER BY corrected_at ASC, id ASC;

-- name: InsertCategoryAssignment :exec
INSERT INTO financialcore.category_assignments (
    id, financial_event_id, category, confidence, source, assigned_at, is_current
) VALUES ($1, $2, $3, $4, $5, $6, true);

-- name: SupersedeCurrentCategoryAssignments :exec
UPDATE financialcore.category_assignments
SET is_current = false, superseded_by = @new_id::uuid
WHERE financial_event_id = @event_id::uuid
  AND is_current = true
  AND id <> @new_id::uuid;

-- name: ListCurrentCategoryAssignmentsForEvents :many
SELECT id, financial_event_id, category, confidence::float8 AS confidence, source, assigned_at, superseded_by, is_current
FROM financialcore.category_assignments
WHERE financial_event_id = ANY(@event_ids::uuid[])
  AND is_current = true;

-- name: TryRecordInbox :execrows
INSERT INTO financialcore.inbox_processed (task_id, result_summary)
VALUES ($1, $2)
ON CONFLICT (task_id) DO NOTHING;

-- name: CountPendingReview :one
SELECT count(*) FROM financialcore.financial_events WHERE status = 'pending_review';

-- name: CountPendingReviewCreatedBetween :one
SELECT count(*) FROM financialcore.financial_events
WHERE status = 'pending_review'
  AND created_at >= @not_before::timestamptz
  AND created_at < @not_after::timestamptz;
