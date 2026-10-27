package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// UpdatePreferencesCommand contains fields to update.
type UpdatePreferencesCommand struct {
	UserID                   uuid.UUID
	PhoneNumber              *string
	DailySummaryEnabled      *bool
	WeeklySummaryEnabled     *bool
	MonthlyReportEnabled     *bool
	BudgetAlertsEnabled      *bool
	AiRecommendationsEnabled *bool
}

// UpdatePreferencesHandler executes preference updates.
type UpdatePreferencesHandler struct {
	repo domain.NotificationPreferenceRepository
}

// NewUpdatePreferencesHandler constructs the handler.
func NewUpdatePreferencesHandler(repo domain.NotificationPreferenceRepository) *UpdatePreferencesHandler {
	return &UpdatePreferencesHandler{repo: repo}
}

// Handle executes the update command preserving unmentioned fields per FR-NOTIF-05 [GATE].
func (h *UpdatePreferencesHandler) Handle(ctx context.Context, cmd UpdatePreferencesCommand) (*domain.NotificationPreferences, error) {
	current, err := h.repo.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		current = domain.DefaultPreferences(cmd.UserID)
	}

	if cmd.PhoneNumber != nil {
		current.PhoneNumber = *cmd.PhoneNumber
	}
	if cmd.DailySummaryEnabled != nil {
		current.DailySummaryEnabled = *cmd.DailySummaryEnabled
	}
	if cmd.WeeklySummaryEnabled != nil {
		current.WeeklySummaryEnabled = *cmd.WeeklySummaryEnabled
	}
	if cmd.MonthlyReportEnabled != nil {
		current.MonthlyReportEnabled = *cmd.MonthlyReportEnabled
	}
	if cmd.BudgetAlertsEnabled != nil {
		current.BudgetAlertsEnabled = *cmd.BudgetAlertsEnabled
	}
	if cmd.AiRecommendationsEnabled != nil {
		current.AiRecommendationsEnabled = *cmd.AiRecommendationsEnabled
	}

	if err := h.repo.SavePreferences(ctx, current); err != nil {
		return nil, err
	}

	return current, nil
}
