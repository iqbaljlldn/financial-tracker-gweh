package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// store implements the financialcore persistence ports on top of a sqlc Queries bound to either the
// connection pool (read side) or an active transaction (command side).
type store struct {
	q *gen.Queries
}

// Repository is the pool-bound store used for the read side (EventReader, PendingReviewMonitor).
type Repository struct {
	*store
}

// NewRepository creates the pool-bound read repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{store: &store{q: gen.New(pool)}}
}

// UnitOfWork runs command handlers inside one transaction with transaction-bound repositories.
type UnitOfWork struct {
	pool *pgxpool.Pool
}

// NewUnitOfWork creates a transactional unit of work.
func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

// ExecuteTx runs fn atomically: the state change and its Outbox rows commit or roll back together [GATE].
func (u *UnitOfWork) ExecuteTx(ctx context.Context, fn func(tx *domain.TxRepositories) error) error {
	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("financialcore: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st := &store{q: gen.New(tx)}
	if err := fn(&domain.TxRepositories{Events: st, Outbox: st, Inbox: st}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("financialcore: commit tx: %w", err)
	}
	return nil
}

// --- conversions ---

func toPgUUID(id string) (pgtype.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", id, err)
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

func fromPgUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func toNumeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("invalid numeric %q: %w", s, err)
	}
	return n, nil
}

func toText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func uuidList(ids []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := toPgUUID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// --- EventRepository ---

// InsertIfNew implements the INV-3 idempotent write: ON CONFLICT DO NOTHING on the DB uniqueness
// constraint, so a redelivery is a successful no-op rather than an error [GATE].
func (s *store) InsertIfNew(ctx context.Context, e *domain.FinancialEvent) (bool, error) {
	id, err := toPgUUID(e.ID())
	if err != nil {
		return false, err
	}
	userID, err := toPgUUID(e.UserID())
	if err != nil {
		return false, err
	}
	amount, err := toNumeric(e.Amount().Value())
	if err != nil {
		return false, err
	}
	conf, err := toNumeric(strconv.FormatFloat(e.Confidence().Aggregate, 'f', 3, 64))
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(e.TypeSpecificPayload())
	if err != nil {
		return false, fmt.Errorf("marshaling type-specific payload: %w", err)
	}
	prov := e.Provenance()
	accountID, err := toPgUUID(prov.ConnectorAccountID())
	if err != nil {
		return false, err
	}

	_, err = s.q.InsertFinancialEventIfNew(ctx, gen.InsertFinancialEventIfNewParams{
		ID:                           id,
		UserID:                       userID,
		EventType:                    string(e.EventType()),
		AmountValue:                  amount,
		AmountCurrency:               e.Amount().Currency(),
		OccurredAt:                   toTimestamptz(e.OccurredAt()),
		MerchantRawName:              toText(e.Merchant().RawName),
		MerchantNormalizedName:       toText(e.Merchant().NormalizedName),
		PaymentMethodType:            toText(e.PaymentMethod().Type),
		PaymentMethodHint:            toText(e.PaymentMethod().Hint),
		Status:                       string(e.Status()),
		ConfidenceAggregate:          conf,
		ConfidenceMethod:             string(e.ConfidenceMethod()),
		TypeSpecificPayload:          payload,
		ProvenanceConnectorType:      prov.ConnectorType(),
		ProvenanceConnectorAccountID: accountID,
		ProvenanceSourceMessageRef:   prov.SourceMessageRef(),
		ProvenanceExtractionMethod:   string(prov.ExtractionMethod()),
		ProvenanceTemplateVersion:    toText(prov.TemplateVersion()),
		ProvenancePromptVersion:      toText(prov.PromptVersion()),
		ProvenanceModelIdentifier:    toText(prov.ModelIdentifier()),
		ProvenanceExtractedAt:        toTimestamptz(prov.ExtractedAt()),
		CreatedAt:                    toTimestamptz(e.CreatedAt()),
		UpdatedAt:                    toTimestamptz(e.UpdatedAt()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // conflict absorbed by uq_financial_events_provenance
	}
	if err != nil {
		return false, fmt.Errorf("inserting financial event: %w", err)
	}
	return true, nil
}

// FindByIDForUpdate loads the aggregate under a row lock (command side).
func (s *store) FindByIDForUpdate(ctx context.Context, userID, id string) (*domain.FinancialEvent, error) {
	pid, uid, err := idPair(id, userID)
	if err != nil {
		return nil, domain.ErrEventNotFound
	}
	row, err := s.q.GetFinancialEventForUpdate(ctx, gen.GetFinancialEventForUpdateParams{ID: pid, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading financial event for update: %w", err)
	}
	return s.hydrateOne(ctx, eventRow(row))
}

// FindByID loads the aggregate without locking (query side).
func (s *store) FindByID(ctx context.Context, userID, id string) (*domain.FinancialEvent, error) {
	pid, uid, err := idPair(id, userID)
	if err != nil {
		return nil, domain.ErrEventNotFound
	}
	row, err := s.q.GetFinancialEvent(ctx, gen.GetFinancialEventParams{ID: pid, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading financial event: %w", err)
	}
	return s.hydrateOne(ctx, eventRow(row))
}

func idPair(id, userID string) (pgtype.UUID, pgtype.UUID, error) {
	pid, err := toPgUUID(id)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	uid, err := toPgUUID(userID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	return pid, uid, nil
}

// UpdateStatus persists the lifecycle status of a loaded aggregate.
func (s *store) UpdateStatus(ctx context.Context, e *domain.FinancialEvent) error {
	id, err := toPgUUID(e.ID())
	if err != nil {
		return err
	}
	return s.q.UpdateFinancialEventStatus(ctx, gen.UpdateFinancialEventStatusParams{
		ID:        id,
		Status:    string(e.Status()),
		UpdatedAt: toTimestamptz(e.UpdatedAt()),
	})
}

// InsertOverride appends an override audit row.
func (s *store) InsertOverride(ctx context.Context, o *domain.EventOverride, actorUserID string) error {
	id, err := toPgUUID(o.ID())
	if err != nil {
		return err
	}
	eventID, err := toPgUUID(o.FinancialEventID())
	if err != nil {
		return err
	}
	actor, err := toPgUUID(actorUserID)
	if err != nil {
		return err
	}
	return s.q.InsertEventOverride(ctx, gen.InsertEventOverrideParams{
		ID:               id,
		FinancialEventID: eventID,
		FieldName:        o.FieldName(),
		PreviousValue:    o.PreviousValue(),
		NewValue:         o.NewValue(),
		ActorUserID:      actor,
		CorrectedAt:      toTimestamptz(o.CorrectedAt()),
	})
}

// SaveCategoryAssignment inserts the new current assignment, then supersedes earlier current ones.
func (s *store) SaveCategoryAssignment(ctx context.Context, eventID string, a *domain.CategoryAssignment) error {
	id, err := toPgUUID(a.ID())
	if err != nil {
		return err
	}
	evID, err := toPgUUID(eventID)
	if err != nil {
		return err
	}
	conf, err := toNumeric(strconv.FormatFloat(a.Confidence(), 'f', 3, 64))
	if err != nil {
		return err
	}
	if err := s.q.InsertCategoryAssignment(ctx, gen.InsertCategoryAssignmentParams{
		ID:               id,
		FinancialEventID: evID,
		Category:         a.Category(),
		Confidence:       conf,
		Source:           a.Source(),
		AssignedAt:       toTimestamptz(a.AssignedAt()),
	}); err != nil {
		return fmt.Errorf("inserting category assignment: %w", err)
	}
	return s.q.SupersedeCurrentCategoryAssignments(ctx, gen.SupersedeCurrentCategoryAssignmentsParams{
		NewID:   id,
		EventID: evID,
	})
}

// ArchiveByIDs archives non-archived events among ids for the user.
func (s *store) ArchiveByIDs(ctx context.Context, userID string, ids []string, now time.Time) ([]string, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}
	list, err := uuidList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ArchiveFinancialEventsByIDs(ctx, gen.ArchiveFinancialEventsByIDsParams{
		Now: toTimestamptz(now), UserID: uid, Ids: list,
	})
	if err != nil {
		return nil, fmt.Errorf("archiving financial events by id: %w", err)
	}
	return uuidStrings(rows), nil
}

// ArchiveByConnectorAccount archives every non-archived event from one connector account.
func (s *store) ArchiveByConnectorAccount(ctx context.Context, userID, connectorAccountID string, now time.Time) ([]string, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}
	acc, err := toPgUUID(connectorAccountID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ArchiveFinancialEventsByConnectorAccount(ctx, gen.ArchiveFinancialEventsByConnectorAccountParams{
		Now: toTimestamptz(now), UserID: uid, ConnectorAccountID: acc,
	})
	if err != nil {
		return nil, fmt.Errorf("archiving financial events by connector account: %w", err)
	}
	return uuidStrings(rows), nil
}

// PermanentlyEraseArchivedEvents permanently deletes archived financial events older than retention window.
func (s *store) PermanentlyEraseArchivedEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	rowsAffected, err := s.q.PermanentlyEraseArchivedEvents(ctx, toTimestamptz(olderThan))
	if err != nil {
		return 0, fmt.Errorf("permanently erasing archived events: %w", err)
	}
	return rowsAffected, nil
}

func uuidStrings(in []pgtype.UUID) []string {
	out := make([]string, 0, len(in))
	for _, u := range in {
		out = append(out, fromPgUUID(u))
	}
	return out
}

// --- OutboxRepository / InboxRepository ---

// Save writes an outbox row with the caller's trace context embedded in the payload at write time [GATE].
func (s *store) Save(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error {
	aggID, err := toPgUUID(aggregateID)
	if err != nil {
		return err
	}
	raw, err := observability.EmbedTraceContext(ctx, payload)
	if err != nil {
		return fmt.Errorf("marshaling outbox payload: %w", err)
	}
	return s.q.InsertFinancialCoreOutboxEvent(ctx, gen.InsertFinancialCoreOutboxEventParams{
		ID:                   pgtype.UUID{Bytes: uuid.New(), Valid: true},
		AggregateType:        aggregateType,
		AggregateID:          aggID,
		EventType:            eventType,
		Payload:              raw,
		PayloadSchemaVersion: 1,
		CreatedAt:            toTimestamptz(time.Now()),
	})
}

// TryRecord guards against duplicate deliveries and double-taps via inbox_processed.task_id.
func (s *store) TryRecord(ctx context.Context, taskID string, summary any) (bool, error) {
	var raw []byte
	if summary != nil {
		b, err := json.Marshal(summary)
		if err != nil {
			return false, fmt.Errorf("marshaling inbox summary: %w", err)
		}
		raw = b
	}
	n, err := s.q.TryRecordInbox(ctx, gen.TryRecordInboxParams{TaskID: taskID, ResultSummary: raw})
	if err != nil {
		return false, fmt.Errorf("recording inbox task: %w", err)
	}
	return n == 1, nil
}

// --- PendingReviewMonitor ---

// CountPendingReview returns the current pending-review backlog size.
func (s *store) CountPendingReview(ctx context.Context) (int, error) {
	n, err := s.q.CountPendingReview(ctx)
	return int(n), err
}

// CountPendingReviewCrossedSLA counts pending events created in [notBefore, notAfter).
func (s *store) CountPendingReviewCrossedSLA(ctx context.Context, notBefore, notAfter time.Time) (int, error) {
	n, err := s.q.CountPendingReviewCreatedBetween(ctx, gen.CountPendingReviewCreatedBetweenParams{
		NotBefore: toTimestamptz(notBefore), NotAfter: toTimestamptz(notAfter),
	})
	return int(n), err
}
