package command

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type UpdateBudgetCommand struct {
	ID              string
	UserID          string
	Scope           string
	PeriodType      string
	ThresholdAmount domain.Money
	AlertThresholds []float64
	Active          bool
}

type UpdateBudgetHandler struct {
	repo domain.BudgetRepository
}

func NewUpdateBudgetHandler(repo domain.BudgetRepository) *UpdateBudgetHandler {
	return &UpdateBudgetHandler{repo: repo}
}

func (h *UpdateBudgetHandler) Handle(ctx context.Context, cmd UpdateBudgetCommand) (*domain.Budget, error) {
	b, err := h.repo.GetByID(ctx, cmd.ID, cmd.UserID)
	if err != nil {
		return nil, err
	}

	if err := b.Update(
		cmd.Scope,
		cmd.PeriodType,
		cmd.ThresholdAmount,
		cmd.AlertThresholds,
		cmd.Active,
		time.Now().UTC(),
	); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}
