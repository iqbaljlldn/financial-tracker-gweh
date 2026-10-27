package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
)

// DBTX models the database transaction or pool interface.
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// AuditLogRepository implements domain.AuditLogRepository and domain.AuditLogger.
type AuditLogRepository struct {
	queries *gen.Queries
}

func NewAuditLogRepository(db DBTX) *AuditLogRepository {
	return &AuditLogRepository{queries: gen.New(db)}
}

var _ domain.AuditLogRepository = (*AuditLogRepository)(nil)
var _ domain.AuditLogger = (*AuditLogRepository)(nil)

func (r *AuditLogRepository) Log(ctx context.Context, userID *string, action, actor string, metadata map[string]any) error {
	entry := domain.NewAuditLogEntry(
		uuid.NewString(),
		userID,
		action,
		actor,
		metadata,
		time.Now().UTC(),
	)
	return r.Insert(ctx, entry)
}

func (r *AuditLogRepository) Insert(ctx context.Context, entry *domain.AuditLogEntry) error {
	idUUID, err := uuid.Parse(entry.ID())
	if err != nil {
		idUUID = uuid.New()
	}

	var userUUID pgtype.UUID
	if entry.UserID() != nil && *entry.UserID() != "" {
		parsed, err := uuid.Parse(*entry.UserID())
		if err == nil {
			userUUID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}

	metadataBytes, err := entry.MetadataJSON()
	if err != nil {
		metadataBytes = []byte("{}")
	}

	params := gen.InsertAuditLogParams{
		ID:         pgtype.UUID{Bytes: idUUID, Valid: true},
		UserID:     userUUID,
		Action:     entry.Action(),
		Actor:      entry.Actor(),
		Metadata:   metadataBytes,
		OccurredAt: pgtype.Timestamptz{Time: entry.OccurredAt(), Valid: true},
	}

	_, err = r.queries.InsertAuditLog(ctx, params)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func (r *AuditLogRepository) List(ctx context.Context, filter domain.AuditLogFilter) ([]*domain.AuditLogEntry, int64, error) {
	limit := int32(filter.Limit)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := int32(filter.Offset)
	if offset < 0 {
		offset = 0
	}

	var userUUID pgtype.UUID
	if filter.UserID != nil && *filter.UserID != "" {
		parsed, err := uuid.Parse(*filter.UserID)
		if err == nil {
			userUUID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}

	var actionText pgtype.Text
	if filter.Action != nil && *filter.Action != "" {
		actionText = pgtype.Text{String: *filter.Action, Valid: true}
	}

	var fromTime pgtype.Timestamptz
	if filter.FromTime != nil && !filter.FromTime.IsZero() {
		fromTime = pgtype.Timestamptz{Time: *filter.FromTime, Valid: true}
	}

	var toTime pgtype.Timestamptz
	if filter.ToTime != nil && !filter.ToTime.IsZero() {
		toTime = pgtype.Timestamptz{Time: *filter.ToTime, Valid: true}
	}

	countParams := gen.CountAuditLogsParams{
		UserID:   userUUID,
		Action:   actionText,
		FromTime: fromTime,
		ToTime:   toTime,
	}
	totalCount, err := r.queries.CountAuditLogs(ctx, countParams)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	listParams := gen.ListAuditLogsParams{
		Limit:    limit,
		Offset:   offset,
		UserID:   userUUID,
		Action:   actionText,
		FromTime: fromTime,
		ToTime:   toTime,
	}
	rows, err := r.queries.ListAuditLogs(ctx, listParams)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}

	entries := make([]*domain.AuditLogEntry, len(rows))
	for i, row := range rows {
		var uid *string
		if row.UserID.Valid {
			uStr := uuid.UUID(row.UserID.Bytes).String()
			uid = &uStr
		}
		var meta map[string]any
		if len(row.Metadata) > 0 {
			_ = json.Unmarshal(row.Metadata, &meta)
		}
		entries[i] = domain.NewAuditLogEntry(
			uuid.UUID(row.ID.Bytes).String(),
			uid,
			row.Action,
			row.Actor,
			meta,
			row.OccurredAt.Time,
		)
	}

	return entries, totalCount, nil
}

// ConnectorHealthRepository implements domain.ConnectorHealthRepository.
type ConnectorHealthRepository struct {
	queries *gen.Queries
}

func NewConnectorHealthRepository(db DBTX) *ConnectorHealthRepository {
	return &ConnectorHealthRepository{queries: gen.New(db)}
}

var _ domain.ConnectorHealthRepository = (*ConnectorHealthRepository)(nil)

func (r *ConnectorHealthRepository) Insert(ctx context.Context, h *domain.ConnectorHealth) error {
	idUUID, _ := uuid.Parse(h.ID)
	if idUUID == uuid.Nil {
		idUUID = uuid.New()
	}
	acctUUID, _ := uuid.Parse(h.ConnectorAccountID)

	params := gen.InsertConnectorHealthParams{
		ID:                 pgtype.UUID{Bytes: idUUID, Valid: true},
		ConnectorType:      h.ConnectorType,
		ConnectorAccountID: pgtype.UUID{Bytes: acctUUID, Valid: true},
		Status:             h.Status,
		ErrorCount:         int32(h.ErrorCount),
		LatencyMs:          int32(h.LatencyMs),
		RecordedAt:         pgtype.Timestamptz{Time: h.RecordedAt, Valid: true},
	}
	_, err := r.queries.InsertConnectorHealth(ctx, params)
	return err
}

func (r *ConnectorHealthRepository) ListLatest(ctx context.Context) ([]*domain.ConnectorHealth, error) {
	rows, err := r.queries.ListLatestConnectorHealth(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.ConnectorHealth, len(rows))
	for i, row := range rows {
		result[i] = &domain.ConnectorHealth{
			ID:                 uuid.UUID(row.ID.Bytes).String(),
			ConnectorType:      row.ConnectorType,
			ConnectorAccountID: uuid.UUID(row.ConnectorAccountID.Bytes).String(),
			Status:             row.Status,
			ErrorCount:         int(row.ErrorCount),
			LatencyMs:          int(row.LatencyMs),
			RecordedAt:         row.RecordedAt.Time,
		}
	}
	return result, nil
}

// ParserMetricsRepository implements domain.ParserMetricsRepository.
type ParserMetricsRepository struct {
	queries *gen.Queries
}

func NewParserMetricsRepository(db DBTX) *ParserMetricsRepository {
	return &ParserMetricsRepository{queries: gen.New(db)}
}

var _ domain.ParserMetricsRepository = (*ParserMetricsRepository)(nil)

func (r *ParserMetricsRepository) Insert(ctx context.Context, m *domain.ParserMetric) error {
	idUUID, _ := uuid.Parse(m.ID)
	if idUUID == uuid.Nil {
		idUUID = uuid.New()
	}

	params := gen.InsertParserMetricParams{
		ID:              pgtype.UUID{Bytes: idUUID, Valid: true},
		SenderPattern:   m.SenderPattern,
		TemplateVersion: int32(m.TemplateVersion),
		TotalParsed:     int32(m.TotalParsed),
		FailedParsed:    int32(m.FailedParsed),
		RecordedAt:      pgtype.Timestamptz{Time: m.RecordedAt, Valid: true},
	}
	_, err := r.queries.InsertParserMetric(ctx, params)
	return err
}

func (r *ParserMetricsRepository) GetAggregated(ctx context.Context) ([]*domain.ParserMetric, error) {
	rows, err := r.queries.GetAggregatedParserMetrics(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.ParserMetric, len(rows))
	for i, row := range rows {
		var failRate float64
		if row.TotalParsed > 0 {
			failRate = float64(row.FailedParsed) / float64(row.TotalParsed)
		}
		result[i] = &domain.ParserMetric{
			SenderPattern:   row.SenderPattern,
			TemplateVersion: int(row.TemplateVersion),
			TotalParsed:     int(row.TotalParsed),
			FailedParsed:    int(row.FailedParsed),
			FailureRate:     failRate,
			SpikeAlert:      failRate > 0.05,
			RecordedAt:      row.LastRecordedAt.(time.Time),
		}
	}
	return result, nil
}

// PromptMetricsRepository implements domain.PromptMetricsRepository.
type PromptMetricsRepository struct {
	queries *gen.Queries
}

func NewPromptMetricsRepository(db DBTX) *PromptMetricsRepository {
	return &PromptMetricsRepository{queries: gen.New(db)}
}

var _ domain.PromptMetricsRepository = (*PromptMetricsRepository)(nil)

func (r *PromptMetricsRepository) Insert(ctx context.Context, m *domain.PromptMetric) error {
	idUUID, _ := uuid.Parse(m.ID)
	if idUUID == uuid.Nil {
		idUUID = uuid.New()
	}

	var numConf pgtype.Numeric
	_ = numConf.Scan(fmt.Sprintf("%.3f", m.AggregateConfidence))

	params := gen.InsertPromptMetricParams{
		ID:                      pgtype.UUID{Bytes: idUUID, Valid: true},
		PromptVersion:           m.PromptVersion,
		ModelIdentifier:         m.ModelIdentifier,
		AggregateConfidence:     numConf,
		SchemaValidationOutcome: m.SchemaValidationOutcome,
		LaterOverridden:         m.LaterOverridden,
		LatencyMs:               int32(m.LatencyMs),
		TokenUsage:              int32(m.TokenUsage),
		RecordedAt:              pgtype.Timestamptz{Time: m.RecordedAt, Valid: true},
	}
	_, err := r.queries.InsertPromptMetric(ctx, params)
	return err
}

func (r *PromptMetricsRepository) GetSummary(ctx context.Context) ([]*domain.PromptMetricsSummary, error) {
	rows, err := r.queries.GetPromptMetricsSummary(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.PromptMetricsSummary, len(rows))
	for i, row := range rows {
		conf, _ := row.AvgConfidence.Float64Value()
		lat, _ := row.AvgLatencyMs.Float64Value()
		result[i] = &domain.PromptMetricsSummary{
			PromptVersion:   row.PromptVersion,
			ModelIdentifier: row.ModelIdentifier,
			TotalCalls:      row.TotalCalls,
			AvgConfidence:   conf.Float64,
			ValidCount:      row.ValidCount,
			InvalidCount:    row.InvalidCount,
			OverriddenCount: row.OverriddenCount,
			AvgLatencyMs:    lat.Float64,
			TotalTokens:     row.TotalTokens,
		}
	}
	return result, nil
}

func (r *PromptMetricsRepository) ListRecent(ctx context.Context, limit int32) ([]*domain.PromptMetric, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.queries.ListRecentPromptMetrics(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.PromptMetric, len(rows))
	for i, row := range rows {
		conf, _ := row.AggregateConfidence.Float64Value()
		result[i] = &domain.PromptMetric{
			ID:                      uuid.UUID(row.ID.Bytes).String(),
			PromptVersion:           row.PromptVersion,
			ModelIdentifier:         row.ModelIdentifier,
			AggregateConfidence:     conf.Float64,
			SchemaValidationOutcome: row.SchemaValidationOutcome,
			LaterOverridden:         row.LaterOverridden,
			LatencyMs:               int(row.LatencyMs),
			TokenUsage:              int(row.TokenUsage),
			RecordedAt:              row.RecordedAt.Time,
		}
	}
	return result, nil
}
