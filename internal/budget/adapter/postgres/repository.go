package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// Repository implements the budget module persistence and spend query ports.
type Repository struct {
	pool  *pgxpool.Pool
	q     *gen.Queries
	goals *SavingsGoalRepository
}

// SavingsGoalRepository implements domain.SavingsGoalRepository.
type SavingsGoalRepository struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewSavingsGoalRepository creates a new pool-bound SavingsGoalRepository.
func NewSavingsGoalRepository(pool *pgxpool.Pool) *SavingsGoalRepository {
	return &SavingsGoalRepository{
		pool: pool,
		q:    gen.New(pool),
	}
}

// NewRepository creates a new pool-bound Repository for the budget module.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:  pool,
		q:     gen.New(pool),
		goals: NewSavingsGoalRepository(pool),
	}
}

func (r *Repository) Goals() *SavingsGoalRepository {
	return r.goals
}

// --- Helper conversions ---

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
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func toDate(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t.UTC(), Valid: true}
}

func toNumeric(val float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	s := fmt.Sprintf("%.4f", val)
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("scan numeric %f: %w", val, err)
	}
	return n, nil
}

func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// --- BudgetRepository implementation ---

func (r *Repository) Create(ctx context.Context, b *domain.Budget) error {
	id, err := toPgUUID(b.ID())
	if err != nil {
		return err
	}
	userID, err := toPgUUID(b.UserID())
	if err != nil {
		return err
	}
	amountVal, err := toNumeric(b.ThresholdAmount().Value)
	if err != nil {
		return err
	}

	thresholdsNumeric := make([]pgtype.Numeric, len(b.AlertThresholds()))
	for i, t := range b.AlertThresholds() {
		num, err := toNumeric(t)
		if err != nil {
			return err
		}
		thresholdsNumeric[i] = num
	}

	_, err = r.q.CreateBudget(ctx, gen.CreateBudgetParams{
		ID:                      id,
		UserID:                  userID,
		Scope:                   b.Scope(),
		PeriodType:              b.PeriodType(),
		ThresholdAmountValue:    amountVal,
		ThresholdAmountCurrency: b.ThresholdAmount().Currency,
		AlertThresholds:         thresholdsNumeric,
		Active:                  b.Active(),
		CreatedAt:               toTimestamptz(b.CreatedAt()),
		UpdatedAt:               toTimestamptz(b.UpdatedAt()),
	})
	return err
}

func (r *Repository) Update(ctx context.Context, b *domain.Budget) error {
	id, err := toPgUUID(b.ID())
	if err != nil {
		return err
	}
	userID, err := toPgUUID(b.UserID())
	if err != nil {
		return err
	}
	amountVal, err := toNumeric(b.ThresholdAmount().Value)
	if err != nil {
		return err
	}

	thresholdsNumeric := make([]pgtype.Numeric, len(b.AlertThresholds()))
	for i, t := range b.AlertThresholds() {
		num, err := toNumeric(t)
		if err != nil {
			return err
		}
		thresholdsNumeric[i] = num
	}

	_, err = r.q.UpdateBudget(ctx, gen.UpdateBudgetParams{
		ID:                      id,
		Scope:                   b.Scope(),
		PeriodType:              b.PeriodType(),
		ThresholdAmountValue:    amountVal,
		ThresholdAmountCurrency: b.ThresholdAmount().Currency,
		AlertThresholds:         thresholdsNumeric,
		Active:                  b.Active(),
		UpdatedAt:               toTimestamptz(b.UpdatedAt()),
		UserID:                  userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrBudgetNotFound
	}
	return err
}

func (r *Repository) GetByID(ctx context.Context, id, userID string) (*domain.Budget, error) {
	pgID, err := toPgUUID(id)
	if err != nil {
		return nil, err
	}
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	row, err := r.q.GetBudgetByID(ctx, gen.GetBudgetByIDParams{
		ID:     pgID,
		UserID: pgUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBudgetNotFound
		}
		return nil, err
	}

	return toDomainBudget(row)
}

func (r *Repository) ListByUserID(ctx context.Context, userID string, includeInactive bool) ([]*domain.Budget, error) {
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	rows, err := r.q.ListBudgetsByUserID(ctx, gen.ListBudgetsByUserIDParams{
		UserID:          pgUserID,
		IncludeInactive: includeInactive,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Budget, 0, len(rows))
	for _, row := range rows {
		b, err := toDomainBudget(row)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

func (r *Repository) ListMatchingBudgets(ctx context.Context, userID, category string) ([]*domain.Budget, error) {
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	categoryScope := "category:" + category
	rows, err := r.q.ListMatchingBudgets(ctx, gen.ListMatchingBudgetsParams{
		UserID:        pgUserID,
		CategoryScope: categoryScope,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Budget, 0, len(rows))
	for _, row := range rows {
		b, err := toDomainBudget(row)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

func (r *Repository) Delete(ctx context.Context, id, userID string) error {
	pgID, err := toPgUUID(id)
	if err != nil {
		return err
	}
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return err
	}

	rows, err := r.q.DeleteBudget(ctx, gen.DeleteBudgetParams{
		ID:     pgID,
		UserID: pgUserID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrBudgetNotFound
	}
	return nil
}

func toDomainBudget(row gen.BudgetBudget) (*domain.Budget, error) {
	thresholds := make([]float64, 0, len(row.AlertThresholds))
	for _, n := range row.AlertThresholds {
		thresholds = append(thresholds, numericToFloat64(n))
	}

	return domain.NewBudget(
		fromPgUUID(row.ID),
		fromPgUUID(row.UserID),
		row.Scope,
		row.PeriodType,
		domain.Money{
			Value:    numericToFloat64(row.ThresholdAmountValue),
			Currency: row.ThresholdAmountCurrency,
		},
		thresholds,
		row.Active,
		row.CreatedAt.Time,
	)
}

// --- FiredAlertRepository implementation ---

func (r *Repository) ListFiredThresholds(ctx context.Context, budgetID string, periodStart time.Time) ([]float64, error) {
	pgBudgetID, err := toPgUUID(budgetID)
	if err != nil {
		return nil, err
	}

	rows, err := r.q.ListFiredAlerts(ctx, gen.ListFiredAlertsParams{
		BudgetID:    pgBudgetID,
		PeriodStart: toDate(periodStart),
	})
	if err != nil {
		return nil, err
	}

	result := make([]float64, 0, len(rows))
	for _, r := range rows {
		result = append(result, numericToFloat64(r))
	}
	return result, nil
}

func (r *Repository) RecordFiredAlert(ctx context.Context, alertID, budgetID string, periodStart time.Time, threshold float64, firedAt time.Time) (bool, error) {
	pgAlertID, err := toPgUUID(alertID)
	if err != nil {
		return false, err
	}
	pgBudgetID, err := toPgUUID(budgetID)
	if err != nil {
		return false, err
	}
	thNum, err := toNumeric(threshold)
	if err != nil {
		return false, err
	}

	rows, err := r.q.RecordFiredAlert(ctx, gen.RecordFiredAlertParams{
		ID:          pgAlertID,
		BudgetID:    pgBudgetID,
		PeriodStart: toDate(periodStart),
		Threshold:   thNum,
		FiredAt:     toTimestamptz(firedAt),
	})
	if err != nil {
		return false, err
	}
	// Return true if row was inserted; false if duplicate (conflict suppressed)
	return rows > 0, nil
}

// --- SavingsGoalRepository implementation ---

func (r *SavingsGoalRepository) Create(ctx context.Context, g *domain.SavingsGoal) error {
	id, err := toPgUUID(g.ID())
	if err != nil {
		return err
	}
	userID, err := toPgUUID(g.UserID())
	if err != nil {
		return err
	}
	targetVal, err := toNumeric(g.TargetAmount().Value)
	if err != nil {
		return err
	}

	_, err = r.q.CreateSavingsGoal(ctx, gen.CreateSavingsGoalParams{
		ID:                   id,
		UserID:               userID,
		TargetAmountValue:    targetVal,
		TargetAmountCurrency: g.TargetAmount().Currency,
		TargetDate:           toDate(g.TargetDate()),
		CreatedAt:            toTimestamptz(g.CreatedAt()),
		UpdatedAt:            toTimestamptz(g.UpdatedAt()),
	})
	return err
}

func (r *SavingsGoalRepository) Update(ctx context.Context, g *domain.SavingsGoal) error {
	id, err := toPgUUID(g.ID())
	if err != nil {
		return err
	}
	userID, err := toPgUUID(g.UserID())
	if err != nil {
		return err
	}
	targetVal, err := toNumeric(g.TargetAmount().Value)
	if err != nil {
		return err
	}

	_, err = r.q.UpdateSavingsGoal(ctx, gen.UpdateSavingsGoalParams{
		ID:                   id,
		TargetAmountValue:    targetVal,
		TargetAmountCurrency: g.TargetAmount().Currency,
		TargetDate:           toDate(g.TargetDate()),
		UpdatedAt:            toTimestamptz(g.UpdatedAt()),
		UserID:               userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrGoalNotFound
	}
	return err
}

func (r *SavingsGoalRepository) GetByID(ctx context.Context, id, userID string) (*domain.SavingsGoal, error) {
	pgID, err := toPgUUID(id)
	if err != nil {
		return nil, err
	}
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	row, err := r.q.GetSavingsGoalByID(ctx, gen.GetSavingsGoalByIDParams{
		ID:     pgID,
		UserID: pgUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrGoalNotFound
		}
		return nil, err
	}

	return domain.NewSavingsGoal(
		fromPgUUID(row.ID),
		fromPgUUID(row.UserID),
		domain.Money{
			Value:    numericToFloat64(row.TargetAmountValue),
			Currency: row.TargetAmountCurrency,
		},
		row.TargetDate.Time,
		row.CreatedAt.Time,
	)
}

func (r *SavingsGoalRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.SavingsGoal, error) {
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	rows, err := r.q.ListSavingsGoalsByUserID(ctx, pgUserID)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.SavingsGoal, 0, len(rows))
	for _, row := range rows {
		g, err := domain.NewSavingsGoal(
			fromPgUUID(row.ID),
			fromPgUUID(row.UserID),
			domain.Money{
				Value:    numericToFloat64(row.TargetAmountValue),
				Currency: row.TargetAmountCurrency,
			},
			row.TargetDate.Time,
			row.CreatedAt.Time,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, nil
}

func (r *SavingsGoalRepository) Delete(ctx context.Context, id, userID string) error {
	pgID, err := toPgUUID(id)
	if err != nil {
		return err
	}
	pgUserID, err := toPgUUID(userID)
	if err != nil {
		return err
	}

	rows, err := r.q.DeleteSavingsGoal(ctx, gen.DeleteSavingsGoalParams{
		ID:     pgID,
		UserID: pgUserID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrGoalNotFound
	}
	return nil
}

// --- OutboxRepository implementation ---

func (r *Repository) RecordOutboxEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error {
	pgID, err := toPgUUID(uuid.New().String())
	if err != nil {
		return err
	}
	pgAggID, err := toPgUUID(aggregateID)
	if err != nil {
		return err
	}

	payloadBytes, err := observability.EmbedTraceContext(ctx, payload)
	if err != nil {
		return fmt.Errorf("budget: embed trace context in outbox: %w", err)
	}

	return r.q.RecordOutboxEvent(ctx, gen.RecordOutboxEventParams{
		ID:            pgID,
		AggregateType: aggregateType,
		AggregateID:   pgAggID,
		EventType:     eventType,
		Payload:       payloadBytes,
		CreatedAt:     toTimestamptz(time.Now().UTC()),
	})
}

// --- SpendProvider implementation ---

func (r *Repository) GetCumulativeSpend(ctx context.Context, userID string, category string, periodStart, periodEnd time.Time) (float64, error) {
	var spend float64
	if category == "" {
		// Global spend across all expense events in period
		err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount_value), 0)::float8
			FROM financialcore.financial_events
			WHERE user_id = $1::uuid
			  AND status = 'finalized'
			  AND event_type = 'Expense'
			  AND occurred_at >= $2::timestamptz
			  AND occurred_at < $3::timestamptz
		`, userID, periodStart, periodEnd).Scan(&spend)
		return spend, err
	}

	// Category-scoped spend
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(fe.amount_value), 0)::float8
		FROM financialcore.financial_events fe
		JOIN financialcore.category_assignments ca ON ca.financial_event_id = fe.id AND ca.is_current = true
		WHERE fe.user_id = $1::uuid
		  AND fe.status = 'finalized'
		  AND fe.event_type = 'Expense'
		  AND LOWER(TRIM(ca.category)) = LOWER(TRIM($2))
		  AND fe.occurred_at >= $3::timestamptz
		  AND fe.occurred_at < $4::timestamptz
	`, userID, category, periodStart, periodEnd).Scan(&spend)
	return spend, err
}

func (r *Repository) GetNetCashFlow(ctx context.Context, userID string) (float64, error) {
	var netFlow float64
	err := r.pool.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(CASE WHEN event_type = 'Income' THEN amount_value ELSE 0 END), 0)::float8 -
			COALESCE(SUM(CASE WHEN event_type = 'Expense' THEN amount_value ELSE 0 END), 0)::float8
		FROM financialcore.financial_events
		WHERE user_id = $1::uuid
		  AND status = 'finalized'
	`, userID).Scan(&netFlow)
	return netFlow, err
}
