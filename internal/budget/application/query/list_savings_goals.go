package query

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type SavingsGoalDTO struct {
	ID                 string       `json:"id"`
	UserID             string       `json:"userId"`
	TargetAmount       domain.Money `json:"targetAmount"`
	TargetDate         string       `json:"targetDate"` // "YYYY-MM-DD"
	CurrentSavings     domain.Money `json:"currentSavings"`
	ProgressPercentage float64      `json:"progressPercentage"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
}

type ListSavingsGoalsQuery struct {
	UserID string
}

type ListSavingsGoalsHandler struct {
	repo          domain.SavingsGoalRepository
	spendProvider domain.SpendProvider
}

func NewListSavingsGoalsHandler(repo domain.SavingsGoalRepository, spendProvider domain.SpendProvider) *ListSavingsGoalsHandler {
	return &ListSavingsGoalsHandler{
		repo:          repo,
		spendProvider: spendProvider,
	}
}

func (h *ListSavingsGoalsHandler) Handle(ctx context.Context, q ListSavingsGoalsQuery) ([]SavingsGoalDTO, error) {
	goals, err := h.repo.ListByUserID(ctx, q.UserID)
	if err != nil {
		return nil, err
	}

	netCashFlow, err := h.spendProvider.GetNetCashFlow(ctx, q.UserID)
	if err != nil {
		netCashFlow = 0.0
	}

	dtos := make([]SavingsGoalDTO, 0, len(goals))
	for _, g := range goals {
		progress := g.CalculateProgress(netCashFlow)
		currentSavingsVal := netCashFlow
		if currentSavingsVal < 0 {
			currentSavingsVal = 0
		}

		dtos = append(dtos, SavingsGoalDTO{
			ID:                 g.ID(),
			UserID:             g.UserID(),
			TargetAmount:       g.TargetAmount(),
			TargetDate:         g.TargetDate().Format("2006-01-02"),
			CurrentSavings:     domain.Money{Value: currentSavingsVal, Currency: g.TargetAmount().Currency},
			ProgressPercentage: progress,
			CreatedAt:          g.CreatedAt(),
			UpdatedAt:          g.UpdatedAt(),
		})
	}

	return dtos, nil
}
