package command

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type CreateBudgetCommand struct {
	UserID          string
	Scope           string
	PeriodType      string
	ThresholdAmount domain.Money
	AlertThresholds []float64
}

type CreateBudgetHandler struct {
	repo domain.BudgetRepository
}

func NewCreateBudgetHandler(repo domain.BudgetRepository) *CreateBudgetHandler {
	return &CreateBudgetHandler{repo: repo}
}

func (h *CreateBudgetHandler) Handle(ctx context.Context, cmd CreateBudgetCommand) (*domain.Budget, error) {
	budgetID := uuid.New().String()
	b, err := domain.NewBudget(
		budgetID,
		cmd.UserID,
		cmd.Scope,
		cmd.PeriodType,
		cmd.ThresholdAmount,
		cmd.AlertThresholds,
		true,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}
