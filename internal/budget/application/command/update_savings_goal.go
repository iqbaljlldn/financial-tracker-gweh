package command

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type UpdateSavingsGoalCommand struct {
	ID           string
	UserID       string
	TargetAmount domain.Money
	TargetDate   time.Time
}

type UpdateSavingsGoalHandler struct {
	repo domain.SavingsGoalRepository
}

func NewUpdateSavingsGoalHandler(repo domain.SavingsGoalRepository) *UpdateSavingsGoalHandler {
	return &UpdateSavingsGoalHandler{repo: repo}
}

func (h *UpdateSavingsGoalHandler) Handle(ctx context.Context, cmd UpdateSavingsGoalCommand) (*domain.SavingsGoal, error) {
	g, err := h.repo.GetByID(ctx, cmd.ID, cmd.UserID)
	if err != nil {
		return nil, err
	}

	if err := g.Update(cmd.TargetAmount, cmd.TargetDate, time.Now().UTC()); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}
