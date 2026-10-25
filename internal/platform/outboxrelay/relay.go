package outboxrelay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// EventRecord models an unrelayed outbox event row across modules.
type EventRecord struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	CreatedAt     time.Time
}

// EventPublisher abstracts the task queue/message broker (e.g. Asynq client).
type EventPublisher interface {
	Publish(ctx context.Context, event EventRecord) error
}

// OutboxStore defines database operations for fetching and acknowledging outbox rows.
type OutboxStore interface {
	FetchUnrelayed(ctx context.Context, batchSize int32) ([]EventRecord, error)
	MarkRelayed(ctx context.Context, ids []string) error
}

// Config configures polling intervals and batch sizes.
type Config struct {
	PollInterval time.Duration
	BatchSize    int32
	Logger       *slog.Logger
}

// Relay coordinates polling outbox_events with SELECT ... FOR UPDATE SKIP LOCKED
// and dispatching to EventPublisher per 06_architecture.md §6.1 [GATE].
type Relay struct {
	cfg       Config
	store     OutboxStore
	publisher EventPublisher
	logger    *slog.Logger

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewRelay constructs a new generic Outbox Relay.
func NewRelay(cfg Config, store OutboxStore, publisher EventPublisher) *Relay {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Relay{
		cfg:       cfg,
		store:     store,
		publisher: publisher,
		logger:    logger,
	}
}

// Start begins asynchronous polling in a background goroutine.
func (r *Relay) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("outboxrelay: relay already running")
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	go r.pollLoop(ctx)
	return nil
}

// Stop gracefully halts the relay poller loop.
func (r *Relay) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.stopCh)
	r.mu.Unlock()

	<-r.doneCh
}

// PollOnce executes a single poll-publish-ack cycle. Useful for synchronous verification in integration tests.
func (r *Relay) PollOnce(ctx context.Context) (int, error) {
	events, err := r.store.FetchUnrelayed(ctx, r.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("outboxrelay: fetch unrelayed: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	relayedIDs := make([]string, 0, len(events))
	for _, evt := range events {
		if err := r.publisher.Publish(ctx, evt); err != nil {
			r.logger.Error("outboxrelay: failed to publish outbox event",
				"event_id", evt.ID,
				"event_type", evt.EventType,
				"error", err,
			)
			// Halt this batch to preserve delivery ordering and retry failed row
			break
		}
		relayedIDs = append(relayedIDs, evt.ID)
	}

	if len(relayedIDs) > 0 {
		if err := r.store.MarkRelayed(ctx, relayedIDs); err != nil {
			return len(relayedIDs), fmt.Errorf("outboxrelay: mark relayed: %w", err)
		}
	}

	return len(relayedIDs), nil
}

func (r *Relay) pollLoop(ctx context.Context) {
	defer close(r.doneCh)
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := r.PollOnce(ctx)
			if err != nil {
				r.logger.Warn("outboxrelay: poll error", "error", err)
			} else if count > 0 {
				r.logger.Debug("outboxrelay: successfully relayed events", "count", count)
			}
		}
	}
}
