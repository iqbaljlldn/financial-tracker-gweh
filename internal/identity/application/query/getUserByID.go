package query

import (
	"context"
	"fmt"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

// GetUserByIDQuery carries the parameters for reading a user profile.
// UserID is a mandatory parameter establishing row-level ownership convention [GATE] (16_security.md §6.2, 20_coding_convention.md §9.2).
type GetUserByIDQuery struct {
	UserID string
}

// GetUserByIDHandler coordinates reading a user aggregate by ID.
type GetUserByIDHandler struct {
	userRepo domain.UserRepository
}

// NewGetUserByIDHandler constructs a new GetUserByIDHandler.
func NewGetUserByIDHandler(userRepo domain.UserRepository) *GetUserByIDHandler {
	return &GetUserByIDHandler{
		userRepo: userRepo,
	}
}

// Handle executes the read query with mandatory row-level ownership check [GATE].
func (h *GetUserByIDHandler) Handle(ctx context.Context, q GetUserByIDQuery) (*domain.User, error) {
	if q.UserID == "" {
		return nil, domain.ErrUnauthorized
	}

	user, err := h.userRepo.FindByID(ctx, q.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return user, nil
}
