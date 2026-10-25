package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

// ListGmailAccountsQuery carries query filters for user-scoped accounts.
// Mandatory row-level ownership convention [GATE]: all user-scoped queries require UserID.
type ListGmailAccountsQuery struct {
	UserID string
}

// ListGmailAccountsHandler retrieves all GmailAccount aggregates owned by a user.
type ListGmailAccountsHandler struct {
	accountRepo domain.GmailAccountRepository
}

// NewListGmailAccountsHandler creates a new ListGmailAccountsHandler.
func NewListGmailAccountsHandler(accountRepo domain.GmailAccountRepository) *ListGmailAccountsHandler {
	return &ListGmailAccountsHandler{
		accountRepo: accountRepo,
	}
}

// Handle executes the user-scoped account retrieval.
func (h *ListGmailAccountsHandler) Handle(ctx context.Context, q ListGmailAccountsQuery) ([]*domain.GmailAccount, error) {
	if strings.TrimSpace(q.UserID) == "" {
		return nil, fmt.Errorf("list gmail accounts: %w", domain.ErrEmptyAccountFields)
	}

	accounts, err := h.accountRepo.ListByUserID(ctx, q.UserID)
	if err != nil {
		return nil, fmt.Errorf("list accounts for user %s: %w", q.UserID, err)
	}

	return accounts, nil
}
