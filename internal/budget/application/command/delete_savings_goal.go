package command

import (
	"context"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type DeleteSavingsGoalCommand struct {
	ID     string
	UserID string
}

type DeleteSavingsGoalHandler struct {
	repo domain.SavingsGoalRepository
}

func NewDeleteSavingsGoalHandler(repo domain.SavingsGoalRepository) *DeleteSavingsGoalHandler {
	return &DeleteSavingsGoalHandler{repo: repo}
}

func (h *DeleteSavingsGoalHandler) Handle(ctx context.Context, cmd DeleteSavingsGoalCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.UserID)
}
