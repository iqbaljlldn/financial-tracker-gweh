package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// GetPreferencesHandler retrieves user's notification preferences.
type GetPreferencesHandler struct {
	repo domain.NotificationPreferenceRepository
}

// NewGetPreferencesHandler constructs the query handler.
func NewGetPreferencesHandler(repo domain.NotificationPreferenceRepository) *GetPreferencesHandler {
	return &GetPreferencesHandler{repo: repo}
}

// Handle executes the query.
func (h *GetPreferencesHandler) Handle(ctx context.Context, userID uuid.UUID) (*domain.NotificationPreferences, error) {
	return h.repo.GetByUserID(ctx, userID)
}
