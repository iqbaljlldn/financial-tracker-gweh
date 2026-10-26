package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/outboxrelay"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// PostgresOutboxStore implements outboxrelay.OutboxStore for understanding schema.
type PostgresOutboxStore struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// NewPostgresOutboxStore creates an OutboxStore for the understanding schema.
func NewPostgresOutboxStore(pool *pgxpool.Pool) *PostgresOutboxStore {
	return &PostgresOutboxStore{
		pool:    pool,
		queries: gen.New(pool),
	}
}

// FetchUnrelayed fetches unprocessed rows with LIMIT.
func (s *PostgresOutboxStore) FetchUnrelayed(ctx context.Context, batchSize int32) ([]outboxrelay.EventRecord, error) {
	rows, err := s.queries.ListUnrelayedUnderstandingOutboxEvents(ctx, batchSize)
	if err != nil {
		return nil, fmt.Errorf("fetch unrelayed understanding outbox events: %w", err)
	}

	records := make([]outboxrelay.EventRecord, len(rows))
	for i, r := range rows {
		records[i] = outboxrelay.EventRecord{
			ID:            uuid.UUID(r.ID.Bytes).String(),
			AggregateType: r.AggregateType,
			AggregateID:   uuid.UUID(r.AggregateID.Bytes).String(),
			EventType:     r.EventType,
			Payload:       r.Payload,
			CreatedAt:     r.CreatedAt.Time,
		}
	}
	return records, nil
}

// MarkRelayed marks the given outbox row IDs as relayed.
func (s *PostgresOutboxStore) MarkRelayed(ctx context.Context, ids []string) error {
	for _, idStr := range ids {
		u, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		if err := s.queries.MarkUnderstandingOutboxRelayed(ctx, gen.MarkUnderstandingOutboxRelayedParams{
			ID: pgtype.UUID{Bytes: u, Valid: true},
			RelayedAt: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

// AsynqEventPublisher translates understanding outbox events into Asynq tasks.
type AsynqEventPublisher struct {
	client *asynq.Client
	logger *slog.Logger
}

// NewAsynqEventPublisher constructs an AsynqEventPublisher.
func NewAsynqEventPublisher(client *asynq.Client, logger *slog.Logger) *AsynqEventPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &AsynqEventPublisher{
		client: client,
		logger: logger,
	}
}

// Publish enqueues the candidate event to Financial Core confidence gate per Diagram 3 & 4.
func (p *AsynqEventPublisher) Publish(ctx context.Context, event outboxrelay.EventRecord) error {
	switch event.EventType {
	case domain.EventFinancialEventCandidateCreated:
		task := asynq.NewTask(taskserver.TaskEvaluateConfidenceGate, event.Payload)
		_, err := p.client.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueFinancialCoreFinalize), asynq.MaxRetry(5))
		if err != nil {
			return fmt.Errorf("enqueue EvaluateConfidenceGate: %w", err)
		}
		p.logger.Info("enqueued EvaluateConfidenceGate from understanding outbox",
			"aggregate_id", event.AggregateID,
			"queue", taskserver.QueueFinancialCoreFinalize,
		)
		return nil

	default:
		p.logger.Debug("unhandled understanding outbox event", "event_type", event.EventType, "aggregate_id", event.AggregateID)
		return nil
	}
}

// UnderstandingOutboxRelay wraps outboxrelay.Relay configured for Understanding.
type UnderstandingOutboxRelay struct {
	relay *outboxrelay.Relay
}

// NewUnderstandingOutboxRelay instantiates an Outbox Relay for the understanding module.
func NewUnderstandingOutboxRelay(pool *pgxpool.Pool, client *asynq.Client, pollInterval time.Duration, logger *slog.Logger) *UnderstandingOutboxRelay {
	store := NewPostgresOutboxStore(pool)
	publisher := NewAsynqEventPublisher(client, logger)

	relay := outboxrelay.NewRelay(outboxrelay.Config{
		PollInterval: pollInterval,
		BatchSize:    50,
		Logger:       logger,
	}, store, publisher)

	return &UnderstandingOutboxRelay{relay: relay}
}

func (r *UnderstandingOutboxRelay) Start(ctx context.Context) error {
	return r.relay.Start(ctx)
}

func (r *UnderstandingOutboxRelay) Stop() {
	r.relay.Stop()
}

func (r *UnderstandingOutboxRelay) PollOnce(ctx context.Context) (int, error) {
	return r.relay.PollOnce(ctx)
}
