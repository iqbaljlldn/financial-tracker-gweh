package query

import (
	"context"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// ListFinancialEventsHandler retrieves a paginated keyset view of ledger events.
// It holds ONLY a domain.EventReader, guaranteeing zero mutating repository calls (asserted in tests).
type ListFinancialEventsHandler struct {
	reader domain.EventReader
}

// NewListFinancialEventsHandler constructs the list handler.
func NewListFinancialEventsHandler(reader domain.EventReader) *ListFinancialEventsHandler {
	return &ListFinancialEventsHandler{reader: reader}
}

// Handle executes the read query.
func (h *ListFinancialEventsHandler) Handle(ctx context.Context, q ListFinancialEventsQuery) (*domain.ListResult, error) {
	return h.reader.List(ctx, q.Filter)
}

// GetFinancialEventByIDHandler retrieves a single event by ID for the user.
// It holds ONLY a domain.EventReader, guaranteeing zero mutating repository calls (asserted in tests).
type GetFinancialEventByIDHandler struct {
	reader domain.EventReader
}

// NewGetFinancialEventByIDHandler constructs the get handler.
func NewGetFinancialEventByIDHandler(reader domain.EventReader) *GetFinancialEventByIDHandler {
	return &GetFinancialEventByIDHandler{reader: reader}
}

// Handle executes the lookup query.
func (h *GetFinancialEventByIDHandler) Handle(ctx context.Context, q GetFinancialEventByIDQuery) (*domain.FinancialEvent, error) {
	if q.UserID == "" {
		return nil, domain.ErrUserIDRequired
	}
	if q.EventID == "" {
		return nil, domain.ErrEventNotFound
	}
	return h.reader.FindByID(ctx, q.UserID, q.EventID)
}

// ListEventOverridesHandler retrieves the override audit trail for an event.
// It holds ONLY a domain.EventReader, guaranteeing zero mutating repository calls (asserted in tests).
type ListEventOverridesHandler struct {
	reader domain.EventReader
}

// NewListEventOverridesHandler constructs the overrides list handler.
func NewListEventOverridesHandler(reader domain.EventReader) *ListEventOverridesHandler {
	return &ListEventOverridesHandler{reader: reader}
}

// Handle executes the override audit trail query.
func (h *ListEventOverridesHandler) Handle(ctx context.Context, q ListEventOverridesQuery) ([]*domain.EventOverride, error) {
	if q.UserID == "" {
		return nil, domain.ErrUserIDRequired
	}
	if q.EventID == "" {
		return nil, domain.ErrEventNotFound
	}
	return h.reader.ListOverrides(ctx, q.UserID, q.EventID)
}
