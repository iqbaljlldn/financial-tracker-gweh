package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository implements domain.SenderTemplateRepository, domain.ExtractionResultRepository,
// and domain.OutboxRepository using pgxpool and generated sqlc queries.
type Repository struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// NewRepository creates a new Postgres repository for understanding module.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: gen.New(pool),
	}
}

// FindActiveByPattern returns the active template matching the exact pattern.
func (r *Repository) FindActiveByPattern(ctx context.Context, pattern string) (*domain.SenderTemplate, error) {
	row, err := r.queries.GetActiveTemplateByPattern(ctx, pattern)
	if err != nil {
		return nil, domain.ErrTemplateNotFound
	}
	return toDomainTemplate(row)
}

// ListActive returns all currently active templates.
func (r *Repository) ListActive(ctx context.Context) ([]*domain.SenderTemplate, error) {
	rows, err := r.queries.ListActiveTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active templates: %w", err)
	}

	var templates []*domain.SenderTemplate
	for _, row := range rows {
		t, err := toDomainTemplate(row)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// FindByID returns a template by UUID.
func (r *Repository) FindByID(ctx context.Context, id string) (*domain.SenderTemplate, error) {
	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid template id: %w", err)
	}

	row, err := r.queries.GetTemplateByID(ctx, pgtype.UUID{Bytes: parsedUUID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	return toDomainTemplate(row)
}

// GetLatestVersionForPattern returns highest version number for a sender pattern.
func (r *Repository) GetLatestVersionForPattern(ctx context.Context, pattern string) (int, error) {
	maxVer, err := r.queries.GetLatestVersionForPattern(ctx, pattern)
	if err != nil {
		return 0, fmt.Errorf("getting latest version: %w", err)
	}
	return int(maxVer), nil
}

// PublishAtomic implements the atomic template swap operation [GATE per 13_email_parser.md §6.2]:
// In a single transaction:
// 1. Deactivates previously active version for this senderPattern (active = false).
// 2. Inserts new version with active = true.
// Old versions are never deleted to preserve audit trails.
func (r *Repository) PublishAtomic(ctx context.Context, newTemplate *domain.SenderTemplate) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction for template publish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	// 1. Deactivate existing active templates for this pattern
	if err := qtx.DeactivateTemplatesForPattern(ctx, newTemplate.SenderPattern()); err != nil {
		return fmt.Errorf("deactivating previous active templates: %w", err)
	}

	// 2. Serialize rules to JSONB
	rulesBytes, err := json.Marshal(newTemplate.Rules())
	if err != nil {
		return fmt.Errorf("marshaling extraction rules: %w", err)
	}

	parsedID, err := uuid.Parse(newTemplate.ID())
	if err != nil {
		parsedID = uuid.New()
	}

	// 3. Insert new active version
	_, err = qtx.InsertTemplate(ctx, gen.InsertTemplateParams{
		ID:              pgtype.UUID{Bytes: parsedID, Valid: true},
		SenderPattern:   newTemplate.SenderPattern(),
		Version:         int32(newTemplate.Version()),
		ExtractionRules: rulesBytes,
		Active:          true,
		CreatedAt:       pgtype.Timestamptz{Time: newTemplate.CreatedAt(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("inserting new active template: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing template publish transaction: %w", err)
	}

	return nil
}

// SaveResult persists an extraction outcome into understanding.extraction_results.
func (r *Repository) SaveResult(ctx context.Context, result *domain.ExtractionResultRecord) error {
	resultID, err := uuid.Parse(result.ID)
	if err != nil {
		resultID = uuid.New()
	}

	msgID, err := uuid.Parse(result.IngestedMessageID)
	if err != nil {
		return fmt.Errorf("invalid ingested message id: %w", err)
	}

	candidateBytes, err := json.Marshal(result.CandidatePayload)
	if err != nil {
		return fmt.Errorf("marshaling candidate payload: %w", err)
	}

	var confPerFieldBytes []byte
	if len(result.ConfidencePerField) > 0 {
		confPerFieldBytes, _ = json.Marshal(result.ConfidencePerField)
	}

	numericConf := floatToNumeric(result.ConfidenceAggregate)

	var tmplVer pgtype.Text
	if result.TemplateVersion != nil {
		tmplVer = pgtype.Text{String: *result.TemplateVersion, Valid: true}
	}
	var promptVer pgtype.Text
	if result.PromptVersion != nil {
		promptVer = pgtype.Text{String: *result.PromptVersion, Valid: true}
	}
	var modelID pgtype.Text
	if result.ModelIdentifier != nil {
		modelID = pgtype.Text{String: *result.ModelIdentifier, Valid: true}
	}

	createdAt := result.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err = r.queries.InsertExtractionResult(ctx, gen.InsertExtractionResultParams{
		ID:                  pgtype.UUID{Bytes: resultID, Valid: true},
		IngestedMessageID:   pgtype.UUID{Bytes: msgID, Valid: true},
		Method:              result.Method,
		TemplateVersion:     tmplVer,
		PromptVersion:       promptVer,
		ModelIdentifier:     modelID,
		CandidatePayload:    candidateBytes,
		ConfidenceAggregate: numericConf,
		ConfidencePerField:  confPerFieldBytes,
		CreatedAt:           pgtype.Timestamptz{Time: createdAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("inserting extraction result: %w", err)
	}

	return nil
}

// ListRecent returns recent extraction results up to limit.
func (r *Repository) ListRecent(ctx context.Context, limit int) ([]*domain.ExtractionResultRecord, error) {
	rows, err := r.queries.ListRecentExtractionResults(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("listing recent extraction results: %w", err)
	}

	var results []*domain.ExtractionResultRecord
	for _, row := range rows {
		rec, err := toDomainExtractionRecord(row)
		if err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, nil
}

// GetTemplateStatsInWindow aggregates total and failed executions for a template version since timestamp.
func (r *Repository) GetTemplateStatsInWindow(
	ctx context.Context,
	templateVersion string,
	since time.Time,
) (int, int, error) {
	row, err := r.queries.GetTemplateStatsInWindow(ctx, gen.GetTemplateStatsInWindowParams{
		TemplateVersion: pgtype.Text{String: templateVersion, Valid: true},
		CreatedAt:       pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("querying template stats: %w", err)
	}

	return int(row.TotalCount), int(row.FailedCount), nil
}

// RecordOutboxEvent persists an outbox event into understanding.outbox_events.
func (r *Repository) RecordOutboxEvent(
	ctx context.Context,
	aggregateType, aggregateID, eventType string,
	payload any,
) error {
	aggUUID, err := uuid.Parse(aggregateID)
	if err != nil {
		aggUUID = uuid.New()
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling outbox payload: %w", err)
	}

	_, err = r.queries.InsertUnderstandingOutboxEvent(ctx, gen.InsertUnderstandingOutboxEventParams{
		ID:                   pgtype.UUID{Bytes: uuid.New(), Valid: true},
		AggregateType:        aggregateType,
		AggregateID:          pgtype.UUID{Bytes: aggUUID, Valid: true},
		EventType:            eventType,
		Payload:              payloadBytes,
		PayloadSchemaVersion: 1,
		CreatedAt:            pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("inserting understanding outbox event: %w", err)
	}

	return nil
}

func toDomainTemplate(row gen.UnderstandingSenderTemplate) (*domain.SenderTemplate, error) {
	var rules domain.ExtractionRules
	if err := json.Unmarshal(row.ExtractionRules, &rules); err != nil {
		return nil, fmt.Errorf("unmarshaling extraction rules for template %s: %w", row.ID.String(), err)
	}

	return domain.NewSenderTemplate(
		row.ID.String(),
		row.SenderPattern,
		int(row.Version),
		rules,
		row.Active,
		row.CreatedAt.Time,
	)
}

func toDomainExtractionRecord(row gen.UnderstandingExtractionResult) (*domain.ExtractionResultRecord, error) {
	var candidate domain.CandidateFinancialEvent
	if err := json.Unmarshal(row.CandidatePayload, &candidate); err != nil {
		return nil, fmt.Errorf("unmarshaling candidate event: %w", err)
	}

	var confPerField map[string]float64
	if len(row.ConfidencePerField) > 0 {
		_ = json.Unmarshal(row.ConfidencePerField, &confPerField)
	}

	confAgg := numericToFloat(row.ConfidenceAggregate)

	var tmplVer *string
	if row.TemplateVersion.Valid {
		v := row.TemplateVersion.String
		tmplVer = &v
	}
	var promptVer *string
	if row.PromptVersion.Valid {
		v := row.PromptVersion.String
		promptVer = &v
	}
	var modelID *string
	if row.ModelIdentifier.Valid {
		v := row.ModelIdentifier.String
		modelID = &v
	}

	return &domain.ExtractionResultRecord{
		ID:                  row.ID.String(),
		IngestedMessageID:   row.IngestedMessageID.String(),
		Method:              row.Method,
		TemplateVersion:     tmplVer,
		PromptVersion:       promptVer,
		ModelIdentifier:     modelID,
		CandidatePayload:    candidate,
		ConfidenceAggregate: confAgg,
		ConfidencePerField:  confPerField,
		CreatedAt:           row.CreatedAt.Time,
	}, nil
}

func floatToNumeric(f float64) pgtype.Numeric {
	intVal := int64(f * 1000)
	return pgtype.Numeric{
		Int:   big.NewInt(intVal),
		Exp:   -3,
		Valid: true,
	}
}

func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid || n.Int == nil {
		return 0.0
	}
	f, _ := n.Float64Value()
	if f.Valid {
		return f.Float64
	}
	return 0.0
}
