package projection

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

// ReplayOutboxEvent represents an event pulled from outbox history for replay.
type ReplayOutboxEvent struct {
	ID        string
	EventType string
	Payload   []byte
	CreatedAt time.Time
}

// Rebuilder rebuilds all analytics read-models from the canonical financial core outbox history.
// Per SRS Section 6.3, read models must be completely rebuildable to produce identical state.
type Rebuilder struct {
	repo      domain.Repository
	projector *Projector
}

// NewRebuilder constructs a Rebuilder.
func NewRebuilder(repo domain.Repository, projector *Projector) *Rebuilder {
	return &Rebuilder{
		repo:      repo,
		projector: projector,
	}
}

// Rebuild wipes all analytics projections and replays events from the provided history.
func (r *Rebuilder) Rebuild(ctx context.Context, events []ReplayOutboxEvent) error {
	// 1. Wipe all analytics views, projected events, and checkpoints
	if err := r.repo.WipeAnalyticsData(ctx); err != nil {
		return fmt.Errorf("wipe analytics data: %w", err)
	}

	// 2. Replay all events through projector in chronological order
	for _, evt := range events {
		if err := r.projector.ProjectAnalyticsReadModels(ctx, evt.ID, evt.EventType, evt.Payload, evt.CreatedAt); err != nil {
			return fmt.Errorf("replaying event %s (%s): %w", evt.ID, evt.EventType, err)
		}
	}

	return nil
}

// RebuildFromDB replays all relevant outbox events from the database outbox history.
func (r *Rebuilder) RebuildFromDB(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		SELECT id, event_type, payload, created_at
		FROM financialcore.outbox_events
		WHERE event_type IN (
			'FinancialEventFinalized',
			'FinancialEventOverridden',
			'FinancialEventArchived',
			'CategoryAssignmentChanged'
		)
		ORDER BY created_at ASC
	`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("querying financialcore outbox events for rebuild: %w", err)
	}
	defer rows.Close()

	var events []ReplayOutboxEvent
	for rows.Next() {
		var id pgtype.UUID
		var eventType string
		var payload []byte
		var createdAt pgtype.Timestamptz

		if err := rows.Scan(&id, &eventType, &payload, &createdAt); err != nil {
			return fmt.Errorf("scan outbox event: %w", err)
		}

		events = append(events, ReplayOutboxEvent{
			ID:        id.String(),
			EventType: eventType,
			Payload:   payload,
			CreatedAt: createdAt.Time,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration: %w", err)
	}

	return r.Rebuild(ctx, events)
}
