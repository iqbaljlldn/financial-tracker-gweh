package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// Projector updates analytics read-model projections from financial events.
// It is strictly idempotent and resilient to replay.
type Projector struct {
	repo domain.Repository
}

// NewProjector creates a new Projector.
func NewProjector(repo domain.Repository) *Projector {
	return &Projector{repo: repo}
}

// Wire structures for outbox payload unmarshaling
type moneyWire struct {
	Value    json.RawMessage `json:"value"`
	Currency string          `json:"currency"`
}

func parseMoneyValue(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("empty money value")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseFloat(s, 64)
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, nil
	}
	return 0, fmt.Errorf("invalid money value: %s", string(raw))
}

type financialEventFinalizedPayload struct {
	EventID             string    `json:"eventId"`
	UserID              string    `json:"userId"`
	EventType           string    `json:"eventType"`
	Amount              moneyWire `json:"amount"`
	OccurredAt          time.Time `json:"occurredAt"`
	Merchant            string    `json:"merchant"`
	Category            string    `json:"category,omitempty"`
	ConfidenceMethod    string    `json:"confidenceMethod"`
	ConfidenceAggregate float64   `json:"confidenceAggregate"`
	FinalizedVia        string    `json:"finalizedVia"`
	EmittedAt           time.Time `json:"emittedAt"`
}

type overrideRefWire struct {
	OverrideID    string          `json:"overrideId"`
	FieldName     string          `json:"fieldName"`
	PreviousValue json.RawMessage `json:"previousValue"`
	NewValue      json.RawMessage `json:"newValue"`
}

type financialEventOverriddenPayload struct {
	EventID   string            `json:"eventId"`
	UserID    string            `json:"userId"`
	Status    string            `json:"status"`
	Overrides []overrideRefWire `json:"overrides"`
	EmittedAt time.Time         `json:"emittedAt"`
}

type financialEventArchivedPayload struct {
	EventID   string    `json:"eventId"`
	UserID    string    `json:"userId"`
	Reason    string    `json:"reason"`
	EmittedAt time.Time `json:"emittedAt"`
}

type categoryAssignmentChangedPayload struct {
	EventID          string    `json:"eventId"`
	UserID           string    `json:"userId"`
	Category         string    `json:"category"`
	PreviousCategory string    `json:"previousCategory,omitempty"`
	Source           string    `json:"source"`
	Confidence       float64   `json:"confidence"`
	Merchant         string    `json:"merchant"`
	EmittedAt        time.Time `json:"emittedAt"`
}

// ProjectAnalyticsReadModels processes a domain event from financialcore outbox and updates read models.
func (p *Projector) ProjectAnalyticsReadModels(ctx context.Context, outboxEventID string, eventType string, payloadBytes []byte, createdAt time.Time) error {
	now := time.Now().UTC()

	defer func() {
		m := observability.GetMetrics()
		if m != nil {
			if !createdAt.IsZero() {
				lag := time.Since(createdAt).Seconds()
				if lag < 0 {
					lag = 0
				}
				m.AnalyticsProjectionLagSeconds.WithLabelValues("all").Observe(lag)
			}
			m.AnalyticsProjectionCheckpointAgeSeconds.WithLabelValues("financialcore").Set(0)
		}
	}()

	switch eventType {
	case "FinancialEventFinalized":
		return p.handleFinalized(ctx, outboxEventID, payloadBytes, now)
	case "FinancialEventArchived":
		return p.handleArchived(ctx, outboxEventID, payloadBytes, now)
	case "CategoryAssignmentChanged":
		return p.handleCategoryChanged(ctx, outboxEventID, payloadBytes, now)
	case "FinancialEventOverridden":
		return p.handleOverridden(ctx, outboxEventID, payloadBytes, now)
	default:
		slog.DebugContext(ctx, "analytics projector: ignoring unhandled event type", "eventType", eventType)
		// Still advance checkpoint so we do not re-process unknown events
		return p.repo.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
	}
}

func (p *Projector) handleFinalized(ctx context.Context, outboxEventID string, payloadBytes []byte, now time.Time) error {
	var payload financialEventFinalizedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("unmarshal FinancialEventFinalized payload: %w", err)
	}

	amountVal, err := parseMoneyValue(payload.Amount.Value)
	if err != nil {
		return fmt.Errorf("parse finalized amount: %w", err)
	}
	currency := strings.TrimSpace(payload.Amount.Currency)
	if currency == "" {
		return fmt.Errorf("missing currency in finalized event %s", payload.EventID)
	}

	occurredAt := payload.OccurredAt.UTC()
	dayBucket := time.Date(occurredAt.Year(), occurredAt.Month(), occurredAt.Day(), 0, 0, 0, 0, time.UTC)
	monthBucket := time.Date(occurredAt.Year(), occurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)

	var income, expense, netFlow float64
	isIncome := strings.EqualFold(payload.EventType, "Income")
	isExpense := strings.EqualFold(payload.EventType, "Expense")
	if isIncome {
		income = amountVal
		netFlow = amountVal
	} else if isExpense {
		expense = amountVal
		netFlow = -amountVal
	}

	category := strings.TrimSpace(payload.Category)
	if category == "" {
		category = "Uncategorized"
	}
	merchant := strings.TrimSpace(payload.Merchant)
	if merchant == "" {
		merchant = "Unknown"
	}

	return p.repo.ExecTx(ctx, func(tx domain.Repository) error {
		// Update Cash Flow & Trend (daily + monthly)
		if income > 0 || expense > 0 {
			if err := tx.UpsertCashFlow(ctx, payload.UserID, dayBucket, "daily", income, expense, currency, now); err != nil {
				return fmt.Errorf("upsert cash flow daily: %w", err)
			}
			if err := tx.UpsertCashFlow(ctx, payload.UserID, monthBucket, "monthly", income, expense, currency, now); err != nil {
				return fmt.Errorf("upsert cash flow monthly: %w", err)
			}
			if err := tx.UpsertTrend(ctx, payload.UserID, dayBucket, "daily", netFlow, currency, now); err != nil {
				return fmt.Errorf("upsert trend daily: %w", err)
			}
			if err := tx.UpsertTrend(ctx, payload.UserID, monthBucket, "monthly", netFlow, currency, now); err != nil {
				return fmt.Errorf("upsert trend monthly: %w", err)
			}
		}

		// Update Category Breakdown & Merchant Ranking if Expense
		if isExpense {
			if err := tx.UpsertCategoryBreakdown(ctx, payload.UserID, monthBucket, category, expense, currency, now); err != nil {
				return fmt.Errorf("upsert category breakdown: %w", err)
			}
			if err := tx.RecalculateCategoryPercentageShares(ctx, payload.UserID, monthBucket, currency, now); err != nil {
				return fmt.Errorf("recalculate category percentage shares: %w", err)
			}
			if err := tx.UpsertMerchantRanking(ctx, payload.UserID, monthBucket, merchant, expense, 1, currency, now); err != nil {
				return fmt.Errorf("upsert merchant ranking: %w", err)
			}
		}

		// Save Projected Event record
		pe := domain.ProjectedEvent{
			EventID:                payload.EventID,
			UserID:                 payload.UserID,
			EventType:              payload.EventType,
			AmountValue:            amountVal,
			Currency:               currency,
			OccurredAt:             occurredAt,
			MerchantNormalizedName: merchant,
			Category:               category,
			Status:                 "active",
			UpdatedAt:              now,
		}
		if err := tx.SaveProjectedEvent(ctx, pe); err != nil {
			return fmt.Errorf("save projected event: %w", err)
		}

		// Update checkpoint
		if err := tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now); err != nil {
			return fmt.Errorf("upsert projection checkpoint: %w", err)
		}
		return nil
	})
}

func (p *Projector) handleArchived(ctx context.Context, outboxEventID string, payloadBytes []byte, now time.Time) error {
	var payload financialEventArchivedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("unmarshal FinancialEventArchived payload: %w", err)
	}

	return p.repo.ExecTx(ctx, func(tx domain.Repository) error {
		existing, err := tx.GetProjectedEvent(ctx, payload.EventID)
		if err != nil {
			// If not found in projected_events, event was never projected as finalized
			slog.DebugContext(ctx, "archived event not found in projected events, skipping reversal", "eventId", payload.EventID)
			return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
		}

		if existing != nil && existing.Status == "active" {
			occurredAt := existing.OccurredAt.UTC()
			dayBucket := time.Date(occurredAt.Year(), occurredAt.Month(), occurredAt.Day(), 0, 0, 0, 0, time.UTC)
			monthBucket := time.Date(occurredAt.Year(), occurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)

			var income, expense, netFlow float64
			isIncome := strings.EqualFold(existing.EventType, "Income")
			isExpense := strings.EqualFold(existing.EventType, "Expense")
			if isIncome {
				income = -existing.AmountValue
				netFlow = -existing.AmountValue
			} else if isExpense {
				expense = -existing.AmountValue
				netFlow = existing.AmountValue
			}

			if income != 0 || expense != 0 {
				_ = tx.UpsertCashFlow(ctx, existing.UserID, dayBucket, "daily", income, expense, existing.Currency, now)
				_ = tx.UpsertCashFlow(ctx, existing.UserID, monthBucket, "monthly", income, expense, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, dayBucket, "daily", netFlow, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, monthBucket, "monthly", netFlow, existing.Currency, now)
			}

			if isExpense {
				cat := existing.Category
				if cat == "" {
					cat = "Uncategorized"
				}
				_ = tx.UpsertCategoryBreakdown(ctx, existing.UserID, monthBucket, cat, -existing.AmountValue, existing.Currency, now)
				_ = tx.RecalculateCategoryPercentageShares(ctx, existing.UserID, monthBucket, existing.Currency, now)
				merch := existing.MerchantNormalizedName
				if merch == "" {
					merch = "Unknown"
				}
				_ = tx.UpsertMerchantRanking(ctx, existing.UserID, monthBucket, merch, -existing.AmountValue, -1, existing.Currency, now)
			}

			existing.Status = "archived"
			existing.UpdatedAt = now
			if err := tx.SaveProjectedEvent(ctx, *existing); err != nil {
				return fmt.Errorf("update projected event to archived: %w", err)
			}
		}

		return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
	})
}

func (p *Projector) handleCategoryChanged(ctx context.Context, outboxEventID string, payloadBytes []byte, now time.Time) error {
	var payload categoryAssignmentChangedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("unmarshal CategoryAssignmentChanged payload: %w", err)
	}

	return p.repo.ExecTx(ctx, func(tx domain.Repository) error {
		existing, err := tx.GetProjectedEvent(ctx, payload.EventID)
		if err != nil {
			slog.DebugContext(ctx, "category changed event not found in projected events, skipping", "eventId", payload.EventID)
			return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
		}

		if existing != nil && existing.Status == "active" && strings.EqualFold(existing.EventType, "Expense") {
			monthBucket := time.Date(existing.OccurredAt.UTC().Year(), existing.OccurredAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
			oldCat := strings.TrimSpace(payload.PreviousCategory)
			if oldCat == "" {
				oldCat = existing.Category
			}
			if oldCat == "" {
				oldCat = "Uncategorized"
			}
			newCat := strings.TrimSpace(payload.Category)
			if newCat == "" {
				newCat = "Uncategorized"
			}

			if oldCat != newCat {
				_ = tx.UpsertCategoryBreakdown(ctx, existing.UserID, monthBucket, oldCat, -existing.AmountValue, existing.Currency, now)
				_ = tx.UpsertCategoryBreakdown(ctx, existing.UserID, monthBucket, newCat, existing.AmountValue, existing.Currency, now)
				_ = tx.RecalculateCategoryPercentageShares(ctx, existing.UserID, monthBucket, existing.Currency, now)
				existing.Category = newCat
				existing.UpdatedAt = now
				if err := tx.SaveProjectedEvent(ctx, *existing); err != nil {
					return fmt.Errorf("update projected event category: %w", err)
				}
			}
		}

		return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
	})
}

func (p *Projector) handleOverridden(ctx context.Context, outboxEventID string, payloadBytes []byte, now time.Time) error {
	var payload financialEventOverriddenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("unmarshal FinancialEventOverridden payload: %w", err)
	}

	return p.repo.ExecTx(ctx, func(tx domain.Repository) error {
		existing, err := tx.GetProjectedEvent(ctx, payload.EventID)
		if err != nil {
			slog.DebugContext(ctx, "overridden event not found in projected events, skipping", "eventId", payload.EventID)
			return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
		}

		if existing != nil && existing.Status == "active" {
			// 1. Reverse old contribution
			oldOccurredAt := existing.OccurredAt.UTC()
			oldDayBucket := time.Date(oldOccurredAt.Year(), oldOccurredAt.Month(), oldOccurredAt.Day(), 0, 0, 0, 0, time.UTC)
			oldMonthBucket := time.Date(oldOccurredAt.Year(), oldOccurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)

			var oldIncome, oldExpense, oldNetFlow float64
			if strings.EqualFold(existing.EventType, "Income") {
				oldIncome = -existing.AmountValue
				oldNetFlow = -existing.AmountValue
			} else if strings.EqualFold(existing.EventType, "Expense") {
				oldExpense = -existing.AmountValue
				oldNetFlow = existing.AmountValue
			}

			if oldIncome != 0 || oldExpense != 0 {
				_ = tx.UpsertCashFlow(ctx, existing.UserID, oldDayBucket, "daily", oldIncome, oldExpense, existing.Currency, now)
				_ = tx.UpsertCashFlow(ctx, existing.UserID, oldMonthBucket, "monthly", oldIncome, oldExpense, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, oldDayBucket, "daily", oldNetFlow, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, oldMonthBucket, "monthly", oldNetFlow, existing.Currency, now)
			}

			if strings.EqualFold(existing.EventType, "Expense") {
				cat := existing.Category
				if cat == "" {
					cat = "Uncategorized"
				}
				_ = tx.UpsertCategoryBreakdown(ctx, existing.UserID, oldMonthBucket, cat, -existing.AmountValue, existing.Currency, now)
				_ = tx.RecalculateCategoryPercentageShares(ctx, existing.UserID, oldMonthBucket, existing.Currency, now)
				merch := existing.MerchantNormalizedName
				if merch == "" {
					merch = "Unknown"
				}
				_ = tx.UpsertMerchantRanking(ctx, existing.UserID, oldMonthBucket, merch, -existing.AmountValue, -1, existing.Currency, now)
			}

			// 2. Apply override field modifications
			for _, ref := range payload.Overrides {
				switch ref.FieldName {
				case "amount":
					var mw moneyWire
					if err := json.Unmarshal(ref.NewValue, &mw); err == nil {
						if v, err := parseMoneyValue(mw.Value); err == nil {
							existing.AmountValue = v
						}
						if mw.Currency != "" {
							existing.Currency = mw.Currency
						}
					}
				case "eventType":
					var et string
					if err := json.Unmarshal(ref.NewValue, &et); err == nil && et != "" {
						existing.EventType = et
					}
				case "merchant":
					var m struct {
						RawName        string `json:"rawName"`
						NormalizedName string `json:"normalizedName"`
					}
					if err := json.Unmarshal(ref.NewValue, &m); err == nil {
						name := m.NormalizedName
						if name == "" {
							name = m.RawName
						}
						if name != "" {
							existing.MerchantNormalizedName = name
						}
					} else {
						var s string
						if err := json.Unmarshal(ref.NewValue, &s); err == nil && s != "" {
							existing.MerchantNormalizedName = s
						}
					}
				case "occurredAt":
					var t time.Time
					if err := json.Unmarshal(ref.NewValue, &t); err == nil && !t.IsZero() {
						existing.OccurredAt = t
					}
				}
			}

			// 3. Apply new contribution
			newOccurredAt := existing.OccurredAt.UTC()
			newDayBucket := time.Date(newOccurredAt.Year(), newOccurredAt.Month(), newOccurredAt.Day(), 0, 0, 0, 0, time.UTC)
			newMonthBucket := time.Date(newOccurredAt.Year(), newOccurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)

			var newIncome, newExpense, newNetFlow float64
			if strings.EqualFold(existing.EventType, "Income") {
				newIncome = existing.AmountValue
				newNetFlow = existing.AmountValue
			} else if strings.EqualFold(existing.EventType, "Expense") {
				newExpense = existing.AmountValue
				newNetFlow = -existing.AmountValue
			}

			if newIncome != 0 || newExpense != 0 {
				_ = tx.UpsertCashFlow(ctx, existing.UserID, newDayBucket, "daily", newIncome, newExpense, existing.Currency, now)
				_ = tx.UpsertCashFlow(ctx, existing.UserID, newMonthBucket, "monthly", newIncome, newExpense, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, newDayBucket, "daily", newNetFlow, existing.Currency, now)
				_ = tx.UpsertTrend(ctx, existing.UserID, newMonthBucket, "monthly", newNetFlow, existing.Currency, now)
			}

			if strings.EqualFold(existing.EventType, "Expense") {
				cat := existing.Category
				if cat == "" {
					cat = "Uncategorized"
				}
				_ = tx.UpsertCategoryBreakdown(ctx, existing.UserID, newMonthBucket, cat, existing.AmountValue, existing.Currency, now)
				_ = tx.RecalculateCategoryPercentageShares(ctx, existing.UserID, newMonthBucket, existing.Currency, now)
				merch := existing.MerchantNormalizedName
				if merch == "" {
					merch = "Unknown"
				}
				_ = tx.UpsertMerchantRanking(ctx, existing.UserID, newMonthBucket, merch, existing.AmountValue, 1, existing.Currency, now)
			}

			existing.UpdatedAt = now
			if err := tx.SaveProjectedEvent(ctx, *existing); err != nil {
				return fmt.Errorf("save updated projected event: %w", err)
			}
		}

		return tx.UpsertProjectionCheckpoint(ctx, "financialcore", outboxEventID, now)
	})
}
