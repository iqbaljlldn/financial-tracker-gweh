package query

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type BudgetDTO struct {
	ID              string       `json:"id"`
	UserID          string       `json:"userId"`
	Scope           string       `json:"scope"`
	PeriodType      string       `json:"periodType"`
	ThresholdAmount domain.Money `json:"thresholdAmount"`
	AlertThresholds []float64    `json:"alertThresholds"`
	Active          bool         `json:"active"`
	CurrentSpend    domain.Money `json:"currentSpend"`
	PercentUsed     float64      `json:"percentUsed"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

type ListBudgetsQuery struct {
	UserID          string
	IncludeInactive bool
}

type ListBudgetsHandler struct {
	repo          domain.BudgetRepository
	spendProvider domain.SpendProvider
}

func NewListBudgetsHandler(repo domain.BudgetRepository, spendProvider domain.SpendProvider) *ListBudgetsHandler {
	return &ListBudgetsHandler{
		repo:          repo,
		spendProvider: spendProvider,
	}
}

func (h *ListBudgetsHandler) Handle(ctx context.Context, q ListBudgetsQuery) ([]BudgetDTO, error) {
	budgets, err := h.repo.ListByUserID(ctx, q.UserID, q.IncludeInactive)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	dtos := make([]BudgetDTO, 0, len(budgets))

	for _, b := range budgets {
		var spendCategory string
		if strings.HasPrefix(b.Scope(), "category:") {
			spendCategory = strings.TrimPrefix(b.Scope(), "category:")
		}

		periodStart, periodEnd := b.CalculatePeriodBounds(now)
		currentSpend, err := h.spendProvider.GetCumulativeSpend(ctx, q.UserID, spendCategory, periodStart, periodEnd)
		if err != nil {
			currentSpend = 0.0
		}

		var percentUsed float64
		if b.ThresholdAmount().Value > 0 {
			pct := (currentSpend / b.ThresholdAmount().Value) * 100.0
			percentUsed = math.Round(pct*100) / 100
		}

		dtos = append(dtos, BudgetDTO{
			ID:              b.ID(),
			UserID:          b.UserID(),
			Scope:           b.Scope(),
			PeriodType:      b.PeriodType(),
			ThresholdAmount: b.ThresholdAmount(),
			AlertThresholds: b.AlertThresholds(),
			Active:          b.Active(),
			CurrentSpend: domain.Money{
				Value:    currentSpend,
				Currency: b.ThresholdAmount().Currency,
			},
			PercentUsed: percentUsed,
			CreatedAt:   b.CreatedAt(),
			UpdatedAt:   b.UpdatedAt(),
		})
	}

	return dtos, nil
}
