package command

import (
	"context"
	"fmt"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

// UpdateUserSettingsCommand carries user profile/preference updates.
// Mandates UserID for row-level ownership enforcement [GATE] (16_security.md §6.2, 20_coding_convention.md §9.2).
type UpdateUserSettingsCommand struct {
	UserID                string
	DisplayName           string
	Timezone              string
	NotificationDailyTime *string
}

// UpdateUserSettingsHandler executes the user settings update command.
type UpdateUserSettingsHandler struct {
	userRepo domain.UserRepository
}

// NewUpdateUserSettingsHandler creates a new UpdateUserSettingsHandler.
func NewUpdateUserSettingsHandler(userRepo domain.UserRepository) *UpdateUserSettingsHandler {
	return &UpdateUserSettingsHandler{
		userRepo: userRepo,
	}
}

// Handle verifies ownership and updates user settings per 04_domain_model.md §8.1.
func (h *UpdateUserSettingsHandler) Handle(ctx context.Context, cmd UpdateUserSettingsCommand) (*domain.User, error) {
	if cmd.UserID == "" {
		return nil, domain.ErrUnauthorized
	}

	// 1. Mandatory ownership check: lookup user with authenticated userID [GATE]
	user, err := h.userRepo.FindByID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("update user settings: user lookup failed: %w", err)
	}

	// 2. Apply domain aggregate updates
	user.UpdateSettings(cmd.DisplayName, cmd.Timezone, cmd.NotificationDailyTime)

	// 3. Persist with mandatory userID parameter [GATE]
	if err := h.userRepo.Update(ctx, cmd.UserID, user); err != nil {
		return nil, fmt.Errorf("update user settings: persistence failed: %w", err)
	}

	return user, nil
}
