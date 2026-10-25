package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// RecordPromptMetric records prompt execution metrics into administration.prompt_metrics (11_ai_blueprint.md §10).
func (r *Repository) RecordPromptMetric(ctx context.Context, metric *domain.PromptMetricRecord) error {
	metricID, err := uuid.Parse(metric.ID)
	if err != nil {
		metricID = uuid.New()
	}

	recordedAt := metric.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	numericConf := floatToNumeric(metric.AggregateConfidence)

	const query = `
		INSERT INTO administration.prompt_metrics (
			id, prompt_version, model_identifier, aggregate_confidence,
			schema_validation_outcome, later_overridden, latency_ms, token_usage, recorded_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`

	_, err = r.pool.Exec(ctx, query,
		metricID,
		metric.PromptVersion,
		metric.ModelIdentifier,
		numericConf,
		metric.SchemaValidationOutcome,
		metric.LaterOverridden,
		metric.LatencyMs,
		metric.TokenUsage,
		recordedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting prompt metric: %w", err)
	}

	return nil
}
