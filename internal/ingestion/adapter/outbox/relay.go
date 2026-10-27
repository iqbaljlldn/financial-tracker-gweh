package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/outboxrelay"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// PostgresOutboxStore implements outboxrelay.OutboxStore using Ingestion schema queries.
type PostgresOutboxStore struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// NewPostgresOutboxStore creates an OutboxStore for the ingestion schema.
func NewPostgresOutboxStore(pool *pgxpool.Pool) *PostgresOutboxStore {
	return &PostgresOutboxStore{
		pool:    pool,
		queries: gen.New(pool),
	}
}

// FetchUnrelayed fetches unprocessed rows with SELECT ... FOR UPDATE SKIP LOCKED.
func (s *PostgresOutboxStore) FetchUnrelayed(ctx context.Context, batchSize int32) ([]outboxrelay.EventRecord, error) {
	rows, err := s.queries.FetchUnrelayedOutboxEvents(ctx, batchSize)
	if err != nil {
		return nil, fmt.Errorf("fetch unrelayed outbox events: %w", err)
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
	pgUUIDs := make([]pgtype.UUID, 0, len(ids))
	for _, idStr := range ids {
		u, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		pgUUIDs = append(pgUUIDs, pgtype.UUID{Bytes: u, Valid: true})
	}

	return s.queries.MarkOutboxEventsRelayed(ctx, gen.MarkOutboxEventsRelayedParams{
		Column1: pgUUIDs,
		RelayedAt: pgtype.Timestamptz{
			Time:  time.Now().UTC(),
			Valid: true,
		},
	})
}

// AsynqEventPublisher translates Ingestion outbox events into Asynq tasks.
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

// Publish enqueues the domain event to the appropriate Asynq queue based on event type.
func (p *AsynqEventPublisher) Publish(ctx context.Context, event outboxrelay.EventRecord) error {
	switch event.EventType {
	case domain.EventGmailAccountLinked:
		var payload domain.GmailAccountLinkedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal GmailAccountLinked: %w", err)
		}

		// Enqueue StartInitialFullSync task to ingestion:bulk per 06_architecture.md §6.3
		taskPayload, _ := json.Marshal(map[string]any{
			"accountId":  payload.GmailAccountID,
			"windowDays": 90,
		})

		task := asynq.NewTask(taskserver.TaskStartInitialFullSync, taskPayload)
		_, err := p.client.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueIngestionBulk), asynq.MaxRetry(5))
		if err != nil {
			return fmt.Errorf("enqueue StartInitialFullSync: %w", err)
		}

		p.logger.Info("enqueued StartInitialFullSync from outbox",
			"account_id", payload.GmailAccountID,
			"queue", taskserver.QueueIngestionBulk,
		)
		return nil

	case domain.EventMessageIngested:
		// Enqueue MessageIngested to understanding queues / ingestion:bulk
		task := asynq.NewTask(taskserver.TaskMessageIngested, event.Payload)
		_, err := p.client.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueIngestionBulk), asynq.MaxRetry(5))
		if err != nil {
			return fmt.Errorf("enqueue MessageIngested: %w", err)
		}
		return nil

	case domain.EventUserDataPurgeRequested:
		var payload domain.UserDataPurgeRequestedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal UserDataPurgeRequested: %w", err)
		}

		// Enqueue ArchiveFinancialEvents task to financialcore per 07_sequence_diagrams.md Diagram 8
		taskPayload, _ := json.Marshal(map[string]any{
			"connectorAccountId": payload.GmailAccountID,
			"userId":             payload.UserID,
			"reason":             "gdpr_purge_cascade",
		})

		task := asynq.NewTask(taskserver.TaskArchiveFinancialEvents, taskPayload)
		_, err := p.client.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueFinancialCoreFinalize), asynq.MaxRetry(5))
		if err != nil {
			return fmt.Errorf("enqueue ArchiveFinancialEvents: %w", err)
		}

		p.logger.Info("enqueued ArchiveFinancialEvents from purge outbox event (Diagram 8)",
			"account_id", payload.GmailAccountID,
			"user_id", payload.UserID,
			"queue", taskserver.QueueFinancialCoreFinalize,
		)
		return nil

	default:
		// For other events (e.g. SyncCompleted), enqueue or audit log
		p.logger.Debug("published domain event", "event_type", event.EventType, "aggregate_id", event.AggregateID)
		return nil
	}
}

// IngestionOutboxRelay wraps outboxrelay.Relay configured for Ingestion.
type IngestionOutboxRelay struct {
	relay *outboxrelay.Relay
}

// NewIngestionOutboxRelay instantiates an Outbox Relay for the ingestion module per 06_architecture.md §6.1.
func NewIngestionOutboxRelay(pool *pgxpool.Pool, client *asynq.Client, pollInterval time.Duration, logger *slog.Logger) *IngestionOutboxRelay {
	store := NewPostgresOutboxStore(pool)
	publisher := NewAsynqEventPublisher(client, logger)

	relay := outboxrelay.NewRelay(outboxrelay.Config{
		PollInterval: pollInterval,
		BatchSize:    50,
		Logger:       logger,
	}, store, publisher)

	return &IngestionOutboxRelay{relay: relay}
}

func (r *IngestionOutboxRelay) Start(ctx context.Context) error {
	return r.relay.Start(ctx)
}

func (r *IngestionOutboxRelay) Stop() {
	r.relay.Stop()
}

func (r *IngestionOutboxRelay) PollOnce(ctx context.Context) (int, error) {
	return r.relay.PollOnce(ctx)
}
