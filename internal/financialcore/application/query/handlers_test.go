package query_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// mockReader implements domain.EventReader for unit testing query handlers.
type mockReader struct {
	findFn      func(ctx context.Context, userID, id string) (*domain.FinancialEvent, error)
	listFn      func(ctx context.Context, f domain.ListFilter) (*domain.ListResult, error)
	overridesFn func(ctx context.Context, userID, eventID string) ([]*domain.EventOverride, error)
}

func (m *mockReader) FindByID(ctx context.Context, userID, id string) (*domain.FinancialEvent, error) {
	if m.findFn != nil {
		return m.findFn(ctx, userID, id)
	}
	return nil, domain.ErrEventNotFound
}

func (m *mockReader) List(ctx context.Context, f domain.ListFilter) (*domain.ListResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, f)
	}
	return &domain.ListResult{}, nil
}

func (m *mockReader) ListOverrides(ctx context.Context, userID, eventID string) ([]*domain.EventOverride, error) {
	if m.overridesFn != nil {
		return m.overridesFn(ctx, userID, eventID)
	}
	return nil, nil
}

// Test_QueryHandlers_ZeroMutatingCalls verifies that query handlers have zero mutating calls
// via interface reflection test assertions [GATE: CQRS read-only discipline].
func Test_QueryHandlers_ZeroMutatingCalls(t *testing.T) {
	readerType := reflect.TypeOf((*domain.EventReader)(nil)).Elem()

	mutatingKeywords := []string{
		"Insert", "Update", "Save", "Delete", "Archive", "Mutate", "Write", "Set", "Create", "Upsert",
	}

	for i := 0; i < readerType.NumMethod(); i++ {
		method := readerType.Method(i)
		for _, kw := range mutatingKeywords {
			if strings.HasPrefix(method.Name, kw) {
				t.Fatalf("EventReader interface must NOT contain mutating methods: found %s", method.Name)
			}
		}
	}
}

func Test_QueryHandlers_Execution(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New().String()
	eventID := uuid.New().String()

	mr := &mockReader{
		listFn: func(ctx context.Context, f domain.ListFilter) (*domain.ListResult, error) {
			return &domain.ListResult{
				Items:   nil,
				HasMore: false,
			}, nil
		},
		findFn: func(ctx context.Context, u, id string) (*domain.FinancialEvent, error) {
			if u == userID && id == eventID {
				amt, _ := domain.NewMoney("50.00", "USD")
				now := time.Now().UTC()
				return domain.NewManual(domain.ManualEventParams{
					UserID:       userID,
					EventType:    string(domain.EventTypeExpense),
					Amount:       amt,
					OccurredAt:   now,
					MerchantName: "Store",
				}, 0.70, now)
			}
			return nil, domain.ErrEventNotFound
		},
		overridesFn: func(ctx context.Context, u, id string) ([]*domain.EventOverride, error) {
			return []*domain.EventOverride{}, nil
		},
	}

	listH := query.NewListFinancialEventsHandler(mr)
	res, err := listH.Handle(ctx, query.ListFinancialEventsQuery{
		Filter: domain.ListFilter{UserID: userID},
	})
	if err != nil {
		t.Fatalf("List handler failed: %v", err)
	}
	if res.HasMore {
		t.Fatalf("expected HasMore == false")
	}

	getH := query.NewGetFinancialEventByIDHandler(mr)
	fe, err := getH.Handle(ctx, query.GetFinancialEventByIDQuery{
		UserID:  userID,
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("Get handler failed: %v", err)
	}
	if fe.UserID() != userID {
		t.Fatalf("unexpected user ID: %s", fe.UserID())
	}

	// Missing parameters
	if _, err := getH.Handle(ctx, query.GetFinancialEventByIDQuery{}); err != domain.ErrUserIDRequired {
		t.Fatalf("expected ErrUserIDRequired")
	}
	if _, err := getH.Handle(ctx, query.GetFinancialEventByIDQuery{UserID: userID}); err != domain.ErrEventNotFound {
		t.Fatalf("expected ErrEventNotFound")
	}

	overridesH := query.NewListEventOverridesHandler(mr)
	overrides, err := overridesH.Handle(ctx, query.ListEventOverridesQuery{
		UserID:  userID,
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("ListOverrides handler failed: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("expected 0 overrides")
	}
	if _, err := overridesH.Handle(ctx, query.ListEventOverridesQuery{}); err != domain.ErrUserIDRequired {
		t.Fatalf("expected ErrUserIDRequired")
	}
}
