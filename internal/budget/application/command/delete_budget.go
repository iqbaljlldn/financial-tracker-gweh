package command

import (
	"context"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type DeleteBudgetCommand struct {
	ID     string
	UserID string
}

type DeleteBudgetHandler struct {
	repo domain.BudgetRepository
}

func NewDeleteBudgetHandler(repo domain.BudgetRepository) *DeleteBudgetHandler {
	return &DeleteBudgetHandler{repo: repo}
}

func (h *DeleteBudgetHandler) Handle(ctx context.Context, cmd DeleteBudgetCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.UserID)
}
