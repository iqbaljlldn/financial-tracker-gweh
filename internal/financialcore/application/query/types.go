package query

import (
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// ListFinancialEventsQuery specifies ledger search and keyset pagination criteria.
type ListFinancialEventsQuery struct {
	Filter domain.ListFilter
}

// GetFinancialEventByIDQuery requests a single event by ID scoped to the authenticated user.
type GetFinancialEventByIDQuery struct {
	UserID  string
	EventID string
}

// ListEventOverridesQuery requests the override audit trail for an event.
type ListEventOverridesQuery struct {
	UserID  string
	EventID string
}
