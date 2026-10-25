package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

// DBTX models the query execution capabilities of both pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// IngestionRepository implements GmailAccountRepository, IngestedMessageRepository, OutboxRepository, and UnitOfWork.
type IngestionRepository struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// NewIngestionRepository constructs an IngestionRepository backed by pgxpool.
func NewIngestionRepository(pool *pgxpool.Pool) *IngestionRepository {
	return &IngestionRepository{
		pool:    pool,
		queries: gen.New(pool),
	}
}

// WithTx returns a new repository instance operating within an active pgx.Tx transaction.
func (r *IngestionRepository) WithTx(tx pgx.Tx) *IngestionRepository {
	return &IngestionRepository{
		pool:    r.pool,
		queries: gen.New(tx),
	}
}

func toPgUUID(id string) (pgtype.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

func toPgTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// --- GmailAccountRepository implementation ---

func (r *IngestionRepository) Save(ctx context.Context, account *domain.GmailAccount) error {
	accountUUID, err := toPgUUID(account.ID())
	if err != nil {
		return fmt.Errorf("invalid account uuid: %w", err)
	}

	userUUID, err := toPgUUID(account.UserID())
	if err != nil {
		return fmt.Errorf("invalid user uuid: %w", err)
	}

	err = r.queries.CreateGmailAccount(ctx, gen.CreateGmailAccountParams{
		ID:                    accountUUID,
		UserID:                userUUID,
		EmailAddress:          account.EmailAddress(),
		Status:                string(account.Status()),
		InitialSyncWindowDays: int16(account.InitialSyncWindowDays()),
		CreatedAt:             toPgTimestamptz(account.CreatedAt()),
		UpdatedAt:             toPgTimestamptz(account.UpdatedAt()),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.ErrEmailAlreadyLinked
		}
		return fmt.Errorf("insert gmail account: %w", err)
	}
	return nil
}

func (r *IngestionRepository) Update(ctx context.Context, account *domain.GmailAccount) error {
	accountUUID, err := toPgUUID(account.ID())
	if err != nil {
		return fmt.Errorf("invalid account uuid: %w", err)
	}

	var watchExp pgtype.Timestamptz
	if account.WatchExpiration() != nil {
		watchExp = toPgTimestamptz(*account.WatchExpiration())
	}

	var lastHistoryID pgtype.Text
	if account.LastHistoryID() != "" {
		lastHistoryID = pgtype.Text{
			String: account.LastHistoryID(),
			Valid:  true,
		}
	}

	return r.queries.UpdateGmailAccount(ctx, gen.UpdateGmailAccountParams{
		ID:              accountUUID,
		Status:          string(account.Status()),
		LastHistoryID:   lastHistoryID,
		WatchExpiration: watchExp,
		UpdatedAt:       toPgTimestamptz(account.UpdatedAt()),
	})
}

func (r *IngestionRepository) FindByID(ctx context.Context, id string) (*domain.GmailAccount, error) {
	accountUUID, err := toPgUUID(id)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}

	row, err := r.queries.FindGmailAccountByID(ctx, accountUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("find gmail account by id: %w", err)
	}

	return toDomainGmailAccount(row), nil
}

func (r *IngestionRepository) FindByEmail(ctx context.Context, email string) (*domain.GmailAccount, error) {
	row, err := r.queries.FindGmailAccountByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("find gmail account by email: %w", err)
	}

	return toDomainGmailAccount(row), nil
}

// ListByUserID retrieves all accounts owned by a user, satisfying mandatory userID parameter [GATE].
func (r *IngestionRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.GmailAccount, error) {
	userUUID, err := toPgUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user uuid: %w", err)
	}

	rows, err := r.queries.ListGmailAccountsByUserID(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("list gmail accounts by user id: %w", err)
	}

	accounts := make([]*domain.GmailAccount, len(rows))
	for i, row := range rows {
		accounts[i] = toDomainGmailAccount(row)
	}
	return accounts, nil
}

// --- IngestedMessageRepository implementation ---

func (r *IngestionRepository) SaveBatch(ctx context.Context, messages []*domain.IngestedMessage) error {
	for _, m := range messages {
		msgUUID, err := toPgUUID(m.ID())
		if err != nil {
			return fmt.Errorf("invalid message uuid: %w", err)
		}
		accountUUID, err := toPgUUID(m.GmailAccountID())
		if err != nil {
			return fmt.Errorf("invalid account uuid: %w", err)
		}

		var routing pgtype.Text
		if m.RoutingDecision() != "" {
			routing = pgtype.Text{String: string(m.RoutingDecision()), Valid: true}
		}

		var rawRef pgtype.Text
		if m.RawContentRef() != "" {
			rawRef = pgtype.Text{String: m.RawContentRef(), Valid: true}
		}

		err = r.queries.CreateIngestedMessage(ctx, gen.CreateIngestedMessageParams{
			ID:               msgUUID,
			GmailAccountID:   accountUUID,
			GmailMessageID:   m.GmailMessageID(),
			ReceivedAt:       toPgTimestamptz(m.ReceivedAt()),
			RawContentRef:    rawRef,
			RoutingDecision:  routing,
			ProcessingStatus: string(m.ProcessingStatus()),
			CreatedAt:        toPgTimestamptz(m.CreatedAt()),
		})
		if err != nil {
			return fmt.Errorf("create ingested message: %w", err)
		}
	}
	return nil
}

func (r *IngestionRepository) SaveIfNew(ctx context.Context, m *domain.IngestedMessage) (bool, error) {
	msgUUID, err := toPgUUID(m.ID())
	if err != nil {
		return false, fmt.Errorf("invalid message uuid: %w", err)
	}
	accountUUID, err := toPgUUID(m.GmailAccountID())
	if err != nil {
		return false, fmt.Errorf("invalid account uuid: %w", err)
	}

	var routing pgtype.Text
	if m.RoutingDecision() != "" {
		routing = pgtype.Text{String: string(m.RoutingDecision()), Valid: true}
	}

	var rawRef pgtype.Text
	if m.RawContentRef() != "" {
		rawRef = pgtype.Text{String: m.RawContentRef(), Valid: true}
	}

	_, err = r.queries.CreateIngestedMessageReturning(ctx, gen.CreateIngestedMessageReturningParams{
		ID:               msgUUID,
		GmailAccountID:   accountUUID,
		GmailMessageID:   m.GmailMessageID(),
		ReceivedAt:       toPgTimestamptz(m.ReceivedAt()),
		RawContentRef:    rawRef,
		RoutingDecision:  routing,
		ProcessingStatus: string(m.ProcessingStatus()),
		CreatedAt:        toPgTimestamptz(m.CreatedAt()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Message already exists, idempotent conflict
		}
		return false, fmt.Errorf("create ingested message returning: %w", err)
	}
	return true, nil
}

func (r *IngestionRepository) Exists(ctx context.Context, accountID, gmailMessageID string) (bool, error) {
	accountUUID, err := toPgUUID(accountID)
	if err != nil {
		return false, fmt.Errorf("invalid account uuid: %w", err)
	}

	exists, err := r.queries.CheckIngestedMessageExists(ctx, gen.CheckIngestedMessageExistsParams{
		GmailAccountID: accountUUID,
		GmailMessageID: gmailMessageID,
	})
	if err != nil {
		return false, fmt.Errorf("check message exists: %w", err)
	}
	return exists, nil
}

func (r *IngestionRepository) FindByAccountAndMessageID(ctx context.Context, accountID, gmailMessageID string) (*domain.IngestedMessage, error) {
	accountUUID, err := toPgUUID(accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid account uuid: %w", err)
	}

	row, err := r.queries.FindIngestedMessageByAccountAndMessageID(ctx, gen.FindIngestedMessageByAccountAndMessageIDParams{
		GmailAccountID: accountUUID,
		GmailMessageID: gmailMessageID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMessageNotFound
		}
		return nil, fmt.Errorf("find ingested message: %w", err)
	}

	var rawRef string
	if row.RawContentRef.Valid {
		rawRef = row.RawContentRef.String
	}

	var decision domain.RoutingDecision
	if row.RoutingDecision.Valid {
		decision = domain.RoutingDecision(row.RoutingDecision.String)
	}

	return domain.ReconstituteIngestedMessage(
		uuid.UUID(row.ID.Bytes).String(),
		uuid.UUID(row.GmailAccountID.Bytes).String(),
		row.GmailMessageID,
		row.ReceivedAt.Time,
		rawRef,
		decision,
		domain.ProcessingStatus(row.ProcessingStatus),
		row.CreatedAt.Time,
	), nil
}

// --- OutboxRepository implementation ---

func (r *IngestionRepository) SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error {
	eventID := uuid.New()
	aggUUID, err := toPgUUID(aggregateID)
	if err != nil {
		return fmt.Errorf("invalid aggregate uuid: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	return r.queries.CreateIngestionOutboxEvent(ctx, gen.CreateIngestionOutboxEventParams{
		ID:                   pgtype.UUID{Bytes: eventID, Valid: true},
		AggregateType:        aggregateType,
		AggregateID:          aggUUID,
		EventType:            eventType,
		Payload:              payloadJSON,
		PayloadSchemaVersion: 1,
		CreatedAt:            toPgTimestamptz(time.Now().UTC()),
		Relayed:              false,
	})
}

// --- UnitOfWork implementation ---

func (r *IngestionRepository) ExecuteTx(ctx context.Context, fn func(txRepo *domain.TxRepositories) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := r.WithTx(tx)
	err = fn(&domain.TxRepositories{
		Accounts: txRepo,
		Messages: txRepo,
		Outbox:   txRepo,
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func toDomainGmailAccount(row gen.IngestionGmailAccount) *domain.GmailAccount {
	var watchExp *time.Time
	if row.WatchExpiration.Valid {
		t := row.WatchExpiration.Time
		watchExp = &t
	}

	var lastHistoryID string
	if row.LastHistoryID.Valid {
		lastHistoryID = row.LastHistoryID.String
	}

	return domain.ReconstituteGmailAccount(
		uuid.UUID(row.ID.Bytes).String(),
		uuid.UUID(row.UserID.Bytes).String(),
		row.EmailAddress,
		domain.AccountStatus(row.Status),
		lastHistoryID,
		watchExp,
		int(row.InitialSyncWindowDays),
		row.CreatedAt.Time,
		row.UpdatedAt.Time,
	)
}
