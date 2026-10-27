package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

// Repository implements domain.Repository for the analytics module.
// All operations are confined to the analytics schema per 06_architecture.md §7.
type Repository struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewRepository constructs an Analytics Postgres repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
		q:    gen.New(pool),
	}
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

func toNumeric(val float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	strVal := strconv.FormatFloat(val, 'f', 4, 64)
	if err := n.Scan(strVal); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("converting %v to numeric: %w", val, err)
	}
	return n, nil
}

func numericToString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0.00"
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return "0.00"
	}
	return strconv.FormatFloat(f.Float64, 'f', 2, 64)
}

func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid || n.Int == nil {
		return 0.0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0.0
	}
	return f.Float64
}

func toDate(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{
		Time:  t.UTC(),
		Valid: true,
	}
}

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{
		Time:  t.UTC(),
		Valid: true,
	}
}

// --- Cash Flow ---

func (r *Repository) UpsertCashFlow(ctx context.Context, userID string, bucket time.Time, granularity string, income, expense float64, currency string, updatedAt time.Time) error {
	uid, err := toPgUUID(userID)
	if err != nil {
		return err
	}
	incNum, err := toNumeric(income)
	if err != nil {
		return err
	}
	expNum, err := toNumeric(expense)
	if err != nil {
		return err
	}

	return r.q.UpsertCashFlow(ctx, gen.UpsertCashFlowParams{
		UserID:            uid,
		PeriodBucket:      toDate(bucket),
		Granularity:       granularity,
		TotalIncomeValue:  incNum,
		TotalExpenseValue: expNum,
		Currency:          strings.ToUpper(strings.TrimSpace(currency)),
		UpdatedAt:         toTimestamptz(updatedAt),
	})
}

func (r *Repository) GetCashFlow(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]domain.CashFlowBucket, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	params := gen.GetCashFlowParams{
		UserID:      uid,
		Granularity: granularity,
	}
	if from != nil && !from.IsZero() {
		params.Column3 = toDate(*from)
	}
	if to != nil && !to.IsZero() {
		params.Column4 = toDate(*to)
	}
	if currency != nil && *currency != "" {
		params.Column5 = strings.ToUpper(strings.TrimSpace(*currency))
	}

	rows, err := r.q.GetCashFlow(ctx, params)
	if err != nil {
		return nil, err
	}

	items := make([]domain.CashFlowBucket, len(rows))
	for i, row := range rows {
		items[i] = domain.CashFlowBucket{
			PeriodBucket: row.PeriodBucket.Time.Format("2006-01-02"),
			Granularity:  row.Granularity,
			Currency:     row.Currency,
			TotalIncome:  numericToString(row.TotalIncomeValue),
			TotalExpense: numericToString(row.TotalExpenseValue),
			UpdatedAt:    row.UpdatedAt.Time,
		}
	}
	return items, nil
}

// --- Category Breakdown ---

func (r *Repository) UpsertCategoryBreakdown(ctx context.Context, userID string, bucket time.Time, category string, value float64, currency string, updatedAt time.Time) error {
	uid, err := toPgUUID(userID)
	if err != nil {
		return err
	}
	valNum, err := toNumeric(value)
	if err != nil {
		return err
	}

	category = strings.TrimSpace(category)
	if category == "" {
		category = "Uncategorized"
	}

	var zeroPct pgtype.Numeric
	_ = zeroPct.Scan("0.00")

	return r.q.UpsertCategoryBreakdown(ctx, gen.UpsertCategoryBreakdownParams{
		UserID:          uid,
		PeriodBucket:    toDate(bucket),
		Category:        category,
		TotalValue:      valNum,
		Currency:        strings.ToUpper(strings.TrimSpace(currency)),
		PercentageShare: zeroPct,
		UpdatedAt:       toTimestamptz(updatedAt),
	})
}

func (r *Repository) RecalculateCategoryPercentageShares(ctx context.Context, userID string, bucket time.Time, currency string, updatedAt time.Time) error {
	uid, err := toPgUUID(userID)
	if err != nil {
		return err
	}

	return r.q.RecalculateCategoryPercentageShares(ctx, gen.RecalculateCategoryPercentageSharesParams{
		UserID:       uid,
		PeriodBucket: toDate(bucket),
		Currency:     strings.ToUpper(strings.TrimSpace(currency)),
		UpdatedAt:    toTimestamptz(updatedAt),
	})
}

func (r *Repository) GetCategoryBreakdown(ctx context.Context, userID string, from, to *time.Time, currency *string) ([]domain.CategoryBreakdownItem, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	params := gen.GetCategoryBreakdownParams{
		UserID: uid,
	}
	if from != nil && !from.IsZero() {
		params.Column2 = toDate(*from)
	}
	if to != nil && !to.IsZero() {
		params.Column3 = toDate(*to)
	}
	if currency != nil && *currency != "" {
		params.Column4 = strings.ToUpper(strings.TrimSpace(*currency))
	}

	rows, err := r.q.GetCategoryBreakdown(ctx, params)
	if err != nil {
		return nil, err
	}

	items := make([]domain.CategoryBreakdownItem, len(rows))
	for i, row := range rows {
		items[i] = domain.CategoryBreakdownItem{
			PeriodBucket:    row.PeriodBucket.Time.Format("2006-01-02"),
			Category:        row.Category,
			Currency:        row.Currency,
			TotalValue:      numericToString(row.TotalValue),
			PercentageShare: numericToFloat64(row.PercentageShare),
			UpdatedAt:       row.UpdatedAt.Time,
		}
	}
	return items, nil
}

// --- Merchant Ranking ---

func (r *Repository) UpsertMerchantRanking(ctx context.Context, userID string, bucket time.Time, merchant string, value float64, count int32, currency string, updatedAt time.Time) error {
	uid, err := toPgUUID(userID)
	if err != nil {
		return err
	}
	valNum, err := toNumeric(value)
	if err != nil {
		return err
	}

	merchant = strings.TrimSpace(merchant)
	if merchant == "" {
		merchant = "Unknown Merchant"
	}

	return r.q.UpsertMerchantRanking(ctx, gen.UpsertMerchantRankingParams{
		UserID:                 uid,
		PeriodBucket:           toDate(bucket),
		MerchantNormalizedName: merchant,
		TotalValue:             valNum,
		TransactionCount:       count,
		Currency:               strings.ToUpper(strings.TrimSpace(currency)),
		UpdatedAt:              toTimestamptz(updatedAt),
	})
}

func (r *Repository) GetMerchantRanking(ctx context.Context, userID string, from, to *time.Time, currency *string, limit int32) ([]domain.MerchantRankingItem, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}

	params := gen.GetMerchantRankingParams{
		UserID: uid,
		Limit:  limit,
	}
	if from != nil && !from.IsZero() {
		params.Column2 = toDate(*from)
	}
	if to != nil && !to.IsZero() {
		params.Column3 = toDate(*to)
	}
	if currency != nil && *currency != "" {
		params.Column4 = strings.ToUpper(strings.TrimSpace(*currency))
	}

	rows, err := r.q.GetMerchantRanking(ctx, params)
	if err != nil {
		return nil, err
	}

	items := make([]domain.MerchantRankingItem, len(rows))
	for i, row := range rows {
		items[i] = domain.MerchantRankingItem{
			PeriodBucket:           row.PeriodBucket.Time.Format("2006-01-02"),
			MerchantNormalizedName: row.MerchantNormalizedName,
			Currency:               row.Currency,
			TotalValue:             numericToString(row.TotalValue),
			TransactionCount:       row.TransactionCount,
			UpdatedAt:              row.UpdatedAt.Time,
		}
	}
	return items, nil
}

// --- Trend ---

func (r *Repository) UpsertTrend(ctx context.Context, userID string, bucket time.Time, granularity string, netFlow float64, currency string, updatedAt time.Time) error {
	uid, err := toPgUUID(userID)
	if err != nil {
		return err
	}
	flowNum, err := toNumeric(netFlow)
	if err != nil {
		return err
	}

	return r.q.UpsertTrend(ctx, gen.UpsertTrendParams{
		UserID:       uid,
		PeriodBucket: toDate(bucket),
		Granularity:  granularity,
		NetFlowValue: flowNum,
		Currency:     strings.ToUpper(strings.TrimSpace(currency)),
		UpdatedAt:    toTimestamptz(updatedAt),
	})
}

func (r *Repository) GetTrend(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]domain.TrendItem, error) {
	uid, err := toPgUUID(userID)
	if err != nil {
		return nil, err
	}

	params := gen.GetTrendParams{
		UserID:      uid,
		Granularity: granularity,
	}
	if from != nil && !from.IsZero() {
		params.Column3 = toDate(*from)
	}
	if to != nil && !to.IsZero() {
		params.Column4 = toDate(*to)
	}
	if currency != nil && *currency != "" {
		params.Column5 = strings.ToUpper(strings.TrimSpace(*currency))
	}

	rows, err := r.q.GetTrend(ctx, params)
	if err != nil {
		return nil, err
	}

	items := make([]domain.TrendItem, len(rows))
	for i, row := range rows {
		items[i] = domain.TrendItem{
			PeriodBucket: row.PeriodBucket.Time.Format("2006-01-02"),
			Granularity:  row.Granularity,
			Currency:     row.Currency,
			NetFlow:      numericToString(row.NetFlowValue),
			UpdatedAt:    row.UpdatedAt.Time,
		}
	}
	return items, nil
}

// --- Projection Checkpoint ---

func (r *Repository) UpsertProjectionCheckpoint(ctx context.Context, sourceModule, outboxEventID string, checkpointAt time.Time) error {
	eventID, err := toPgUUID(outboxEventID)
	if err != nil {
		return err
	}

	return r.q.UpsertProjectionCheckpoint(ctx, gen.UpsertProjectionCheckpointParams{
		SourceModule:               sourceModule,
		LastProcessedOutboxEventID: eventID,
		CheckpointAt:               toTimestamptz(checkpointAt),
	})
}

func (r *Repository) GetProjectionCheckpoint(ctx context.Context, sourceModule string) (*domain.ProjectionCheckpoint, error) {
	row, err := r.q.GetProjectionCheckpoint(ctx, sourceModule)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &domain.ProjectionCheckpoint{
		SourceModule:               row.SourceModule,
		LastProcessedOutboxEventID: fromPgUUID(row.LastProcessedOutboxEventID),
		CheckpointAt:               row.CheckpointAt.Time,
	}, nil
}

// --- Projected Events ---

func (r *Repository) SaveProjectedEvent(ctx context.Context, event domain.ProjectedEvent) error {
	eid, err := toPgUUID(event.EventID)
	if err != nil {
		return err
	}
	uid, err := toPgUUID(event.UserID)
	if err != nil {
		return err
	}
	amtNum, err := toNumeric(event.AmountValue)
	if err != nil {
		return err
	}

	return r.q.SaveProjectedEvent(ctx, gen.SaveProjectedEventParams{
		EventID:                eid,
		UserID:                 uid,
		EventType:              event.EventType,
		AmountValue:            amtNum,
		Currency:               strings.ToUpper(strings.TrimSpace(event.Currency)),
		OccurredAt:             toTimestamptz(event.OccurredAt),
		MerchantNormalizedName: event.MerchantNormalizedName,
		Category:               event.Category,
		Status:                 event.Status,
		UpdatedAt:              toTimestamptz(event.UpdatedAt),
	})
}

func (r *Repository) GetProjectedEvent(ctx context.Context, eventID string) (*domain.ProjectedEvent, error) {
	eid, err := toPgUUID(eventID)
	if err != nil {
		return nil, err
	}

	row, err := r.q.GetProjectedEvent(ctx, eid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &domain.ProjectedEvent{
		EventID:                fromPgUUID(row.EventID),
		UserID:                 fromPgUUID(row.UserID),
		EventType:              row.EventType,
		AmountValue:            numericToFloat64(row.AmountValue),
		Currency:               row.Currency,
		OccurredAt:             row.OccurredAt.Time,
		MerchantNormalizedName: row.MerchantNormalizedName,
		Category:               row.Category,
		Status:                 row.Status,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

// --- Wiping ---

func (r *Repository) WipeAnalyticsData(ctx context.Context) error {
	return r.q.WipeAnalyticsData(ctx)
}

// --- Transaction Execution ---

func (r *Repository) ExecTx(ctx context.Context, fn func(repo domain.Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("analytics: begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txRepo := &Repository{
		pool: r.pool,
		q:    r.q.WithTx(tx),
	}

	if err := fn(txRepo); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("analytics: commit transaction: %w", err)
	}
	return nil
}
