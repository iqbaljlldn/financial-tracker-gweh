package command

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type CreateSavingsGoalCommand struct {
	UserID       string
	TargetAmount domain.Money
	TargetDate   time.Time
}

type CreateSavingsGoalHandler struct {
	repo domain.SavingsGoalRepository
}

func NewCreateSavingsGoalHandler(repo domain.SavingsGoalRepository) *CreateSavingsGoalHandler {
	return &CreateSavingsGoalHandler{repo: repo}
}

func (h *CreateSavingsGoalHandler) Handle(ctx context.Context, cmd CreateSavingsGoalCommand) (*domain.SavingsGoal, error) {
	goalID := uuid.New().String()
	g, err := domain.NewSavingsGoal(
		goalID,
		cmd.UserID,
		cmd.TargetAmount,
		cmd.TargetDate,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}
