package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// eventRow is the shared shape of the identical sqlc row structs returned by the get/list queries.
type eventRow = gen.GetFinancialEventRow

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// List returns one keyset page of the user's ledger. userID is mandatory (ErrUserIDRequired).
func (s *store) List(ctx context.Context, f domain.ListFilter) (*domain.ListResult, error) {
	if strings.TrimSpace(f.UserID) == "" {
		return nil, domain.ErrUserIDRequired
	}
	uid, err := toPgUUID(f.UserID)
	if err != nil {
		return nil, domain.ErrUserIDRequired
	}

	limit := f.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	params := gen.ListFinancialEventsParams{
		UserID:    uid,
		PageLimit: int32(limit + 1), // fetch one extra row to compute hasMore
		FromTs:    optTime(f.From),
		ToTs:      optTime(f.To),
		Status:    optText(f.Status),
		EventType: optText(f.EventType),
		Merchant:  optText(f.Merchant),
		Category:  optText(f.Category),
	}
	if f.Cursor != nil {
		cid, err := toPgUUID(f.Cursor.ID)
		if err != nil {
			return nil, domain.ErrInvalidCursor
		}
		params.CursorTs = toTimestamptz(f.Cursor.OccurredAt)
		params.CursorID = cid
	}

	rows, err := s.q.ListFinancialEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("listing financial events: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	converted := make([]eventRow, len(rows))
	for i, r := range rows {
		converted[i] = eventRow(r)
	}
	items, err := s.hydrateMany(ctx, converted)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		res.NextCursor = domain.Cursor{OccurredAt: last.OccurredAt(), ID: last.ID()}.Encode()
	}
	return res, nil
}

// ListOverrides returns the override audit trail oldest-first. The event must belong to userID.
func (s *store) ListOverrides(ctx context.Context, userID, eventID string) ([]*domain.EventOverride, error) {
	pid, uid, err := idPair(eventID, userID)
	if err != nil {
		return nil, domain.ErrEventNotFound
	}
	// Ownership check: a user can never read another user's audit trail.
	if _, err := s.q.GetFinancialEvent(ctx, gen.GetFinancialEventParams{ID: pid, UserID: uid}); err != nil {
		return nil, domain.ErrEventNotFound
	}
	rows, err := s.q.ListEventOverridesForEvent(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("listing event overrides: %w", err)
	}
	out := make([]*domain.EventOverride, 0, len(rows))
	for _, r := range rows {
		out = append(out, toOverride(r))
	}
	return out, nil
}

func optTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return toTimestamptz(*t)
}

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func (s *store) hydrateOne(ctx context.Context, row eventRow) (*domain.FinancialEvent, error) {
	items, err := s.hydrateMany(ctx, []eventRow{row})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// hydrateMany rebuilds aggregates for a page, batch-loading overrides and current categories (2 queries total).
func (s *store) hydrateMany(ctx context.Context, rows []eventRow) ([]*domain.FinancialEvent, error) {
	if len(rows) == 0 {
		return []*domain.FinancialEvent{}, nil
	}

	ids := make([]pgtype.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}

	overrideRows, err := s.q.ListEventOverridesForEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("loading overrides: %w", err)
	}
	overridesByEvent := make(map[string][]*domain.EventOverride)
	for _, o := range overrideRows {
		key := fromPgUUID(o.FinancialEventID)
		overridesByEvent[key] = append(overridesByEvent[key], toOverride(o))
	}

	categoryRows, err := s.q.ListCurrentCategoryAssignmentsForEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("loading category assignments: %w", err)
	}
	categoryByEvent := make(map[string]*domain.CategoryAssignment)
	for _, c := range categoryRows {
		categoryByEvent[fromPgUUID(c.FinancialEventID)] = domain.RehydrateCategoryAssignment(domain.CategoryAssignmentSnapshot{
			ID:           fromPgUUID(c.ID),
			Category:     c.Category,
			Confidence:   c.Confidence,
			Source:       c.Source,
			AssignedAt:   c.AssignedAt.Time,
			SupersededBy: fromPgUUID(c.SupersededBy),
			IsCurrent:    c.IsCurrent,
		})
	}

	out := make([]*domain.FinancialEvent, 0, len(rows))
	for _, r := range rows {
		id := fromPgUUID(r.ID)
		ev, err := toAggregate(r, categoryByEvent[id], overridesByEvent[id])
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func toOverride(o gen.FinancialcoreEventOverride) *domain.EventOverride {
	return domain.RehydrateEventOverride(domain.EventOverrideSnapshot{
		ID:               fromPgUUID(o.ID),
		FinancialEventID: fromPgUUID(o.FinancialEventID),
		FieldName:        o.FieldName,
		PreviousValue:    json.RawMessage(o.PreviousValue),
		NewValue:         json.RawMessage(o.NewValue),
		ActorUserID:      fromPgUUID(o.ActorUserID),
		CorrectedAt:      o.CorrectedAt.Time,
	})
}

func toAggregate(r eventRow, category *domain.CategoryAssignment, overrides []*domain.EventOverride) (*domain.FinancialEvent, error) {
	amount, err := domain.NewMoney(r.AmountValue, r.AmountCurrency)
	if err != nil {
		return nil, fmt.Errorf("rehydrating amount of event %s: %w", fromPgUUID(r.ID), err)
	}

	payload := map[string]any{}
	if len(r.TypeSpecificPayload) > 0 {
		if err := json.Unmarshal(r.TypeSpecificPayload, &payload); err != nil {
			return nil, fmt.Errorf("decoding type-specific payload of event %s: %w", fromPgUUID(r.ID), err)
		}
	}

	return domain.Rehydrate(domain.Snapshot{
		ID:         fromPgUUID(r.ID),
		UserID:     fromPgUUID(r.UserID),
		EventType:  domain.EventType(r.EventType),
		Amount:     amount,
		OccurredAt: r.OccurredAt.Time,
		Merchant: domain.Merchant{
			NormalizedName: r.MerchantNormalizedName.String,
			RawName:        r.MerchantRawName.String,
		},
		PaymentMethod: domain.PaymentMethod{
			Type: r.PaymentMethodType.String,
			Hint: r.PaymentMethodHint.String,
		},
		Status:           domain.Status(r.Status),
		Confidence:       domain.Confidence{Aggregate: r.ConfidenceAggregate},
		ConfidenceMethod: domain.ConfidenceMethod(r.ConfidenceMethod),
		TypeSpecific:     payload,
		Provenance: domain.ProvenanceParams{
			ConnectorType:      r.ProvenanceConnectorType,
			ConnectorAccountID: fromPgUUID(r.ProvenanceConnectorAccountID),
			SourceMessageRef:   r.ProvenanceSourceMessageRef,
			ExtractionMethod:   domain.ExtractionMethod(r.ProvenanceExtractionMethod),
			TemplateVersion:    r.ProvenanceTemplateVersion.String,
			PromptVersion:      r.ProvenancePromptVersion.String,
			ModelIdentifier:    r.ProvenanceModelIdentifier.String,
			ExtractedAt:        r.ProvenanceExtractedAt.Time,
		},
		Category:  category,
		Overrides: overrides,
		CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time,
	}), nil
}
