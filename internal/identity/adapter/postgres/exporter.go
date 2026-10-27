package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

// PostgresUserDataExporter compiles GDPR Article 15/20 user data export archives.
type PostgresUserDataExporter struct {
	db DBTX
}

// NewPostgresUserDataExporter creates a new exporter instance.
func NewPostgresUserDataExporter(db DBTX) *PostgresUserDataExporter {
	return &PostgresUserDataExporter{db: db}
}

// Export queries all user-owned domain data across platform tables and returns a structured export.
// Raw email content (Tier 4) is deliberately excluded per 16_security.md §9.1 and 04_domain_model.md §6.3.
func (e *PostgresUserDataExporter) Export(ctx context.Context, userID string) (*domain.UserDataExport, error) {
	now := time.Now().UTC()

	// 1. User Profile & Settings
	var (
		uID, email             string
		displayName            *string
		timezone               string
		notifDailyTime         *string
		uCreatedAt, uUpdatedAt time.Time
	)
	err := e.db.QueryRow(ctx, `
		SELECT id, email, display_name, timezone, notification_daily_time::text, created_at, updated_at
		FROM identity.users
		WHERE id = $1
	`, userID).Scan(&uID, &email, &displayName, &timezone, &notifDailyTime, &uCreatedAt, &uUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("export user profile: %w", err)
	}

	userMap := map[string]any{
		"id":                    uID,
		"email":                 email,
		"displayName":           displayName,
		"timezone":              timezone,
		"notificationDailyTime": notifDailyTime,
		"createdAt":             uCreatedAt.Format(time.RFC3339),
		"updatedAt":             uUpdatedAt.Format(time.RFC3339),
	}

	// 2. Linked Gmail Accounts (excluding sensitive OAuth credentials per Tier 1)
	linkedAccounts := make([]map[string]any, 0)
	rowsAcc, err := e.db.Query(ctx, `
		SELECT id, email_address, status, last_history_id, created_at, updated_at
		FROM ingestion.gmail_accounts
		WHERE user_id = $1
	`, userID)
	if err == nil {
		defer rowsAcc.Close()
		for rowsAcc.Next() {
			var (
				accID, emailAddr, status, lastHistoryID string
				accCreated, accUpdated                  time.Time
			)
			if scanErr := rowsAcc.Scan(&accID, &emailAddr, &status, &lastHistoryID, &accCreated, &accUpdated); scanErr == nil {
				linkedAccounts = append(linkedAccounts, map[string]any{
					"id":            accID,
					"emailAddress":  emailAddr,
					"status":        status,
					"lastHistoryId": lastHistoryID,
					"createdAt":     accCreated.Format(time.RFC3339),
					"updatedAt":     accUpdated.Format(time.RFC3339),
				})
			}
		}
	}

	// 3. Financial Events
	events := make([]map[string]any, 0)
	rowsEvt, err := e.db.Query(ctx, `
		SELECT id, event_type, amount_value::text, amount_currency, occurred_at,
		       COALESCE(merchant_normalized_name, merchant_raw_name, ''),
		       COALESCE(payment_method_type, ''), COALESCE(payment_method_hint, ''),
		       status, confidence_aggregate, provenance_extraction_method, created_at, updated_at
		FROM financialcore.financial_events
		WHERE user_id = $1
		ORDER BY occurred_at DESC
	`, userID)
	if err == nil {
		defer rowsEvt.Close()
		for rowsEvt.Next() {
			var (
				evID, evType, amtVal, amtCur, merchName, pmType, pmHint, status, extMethod string
				occurredAt, evCreated, evUpdated                                           time.Time
				confidence                                                                 float64
			)
			if scanErr := rowsEvt.Scan(&evID, &evType, &amtVal, &amtCur, &occurredAt,
				&merchName, &pmType, &pmHint, &status, &confidence, &extMethod,
				&evCreated, &evUpdated); scanErr == nil {
				events = append(events, map[string]any{
					"id":                evID,
					"eventType":         evType,
					"amount":            amtVal,
					"currency":          amtCur,
					"occurredAt":        occurredAt.Format(time.RFC3339),
					"merchantName":      merchName,
					"paymentMethodType": pmType,
					"paymentMethodHint": pmHint,
					"status":            status,
					"confidence":        confidence,
					"extractionMethod":  extMethod,
					"createdAt":         evCreated.Format(time.RFC3339),
					"updatedAt":         evUpdated.Format(time.RFC3339),
				})
			}
		}
	}

	// 4. Event Overrides
	overrides := make([]map[string]any, 0)
	rowsOvr, err := e.db.Query(ctx, `
		SELECT id, financial_event_id, field_name, previous_value::text, new_value::text, actor_user_id, corrected_at
		FROM financialcore.event_overrides
		WHERE actor_user_id = $1
		ORDER BY corrected_at DESC
	`, userID)
	if err == nil {
		defer rowsOvr.Close()
		for rowsOvr.Next() {
			var (
				ovrID, evtID, fieldName, prevVal, newVal, actorID string
				ovrCreated                                        time.Time
			)
			if scanErr := rowsOvr.Scan(&ovrID, &evtID, &fieldName, &prevVal, &newVal, &actorID, &ovrCreated); scanErr == nil {
				overrides = append(overrides, map[string]any{
					"id":               ovrID,
					"financialEventId": evtID,
					"fieldName":        fieldName,
					"previousValue":    prevVal,
					"newValue":         newVal,
					"actorUserId":      actorID,
					"createdAt":        ovrCreated.Format(time.RFC3339),
				})
			}
		}
	}

	// 5. Budgets
	budgets := make([]map[string]any, 0)
	rowsBgt, err := e.db.Query(ctx, `
		SELECT id, scope, period_type, threshold_amount_value::text, threshold_amount_currency, active, created_at, updated_at
		FROM budget.budgets
		WHERE user_id = $1
	`, userID)
	if err == nil {
		defer rowsBgt.Close()
		for rowsBgt.Next() {
			var (
				bgtID, scope, period, amtLimit, cur string
				active                              bool
				bgtCreated, bgtUpdated              time.Time
			)
			if scanErr := rowsBgt.Scan(&bgtID, &scope, &period, &amtLimit, &cur, &active, &bgtCreated, &bgtUpdated); scanErr == nil {
				budgets = append(budgets, map[string]any{
					"id":          bgtID,
					"scope":       scope,
					"periodType":  period,
					"amountLimit": amtLimit,
					"currency":    cur,
					"active":      active,
					"createdAt":   bgtCreated.Format(time.RFC3339),
					"updatedAt":   bgtUpdated.Format(time.RFC3339),
				})
			}
		}
	}

	// 6. Notification Preferences
	var preferences any
	var (
		prefUserID                                  string
		dailyEnabled, weeklyEnabled, monthlyEnabled bool
		budgetAlertsEnabled, aiRecsEnabled          bool
	)
	prefErr := e.db.QueryRow(ctx, `
		SELECT user_id, daily_summary_enabled, weekly_summary_enabled,
		       monthly_report_enabled, budget_alerts_enabled, ai_recommendations_enabled
		FROM engagement.notification_preferences
		WHERE user_id = $1
	`, userID).Scan(&prefUserID, &dailyEnabled, &weeklyEnabled, &monthlyEnabled, &budgetAlertsEnabled, &aiRecsEnabled)
	if prefErr == nil {
		preferences = map[string]any{
			"userId":                   prefUserID,
			"dailySummaryEnabled":      dailyEnabled,
			"weeklySummaryEnabled":     weeklyEnabled,
			"monthlyReportEnabled":     monthlyEnabled,
			"budgetAlertsEnabled":      budgetAlertsEnabled,
			"aiRecommendationsEnabled": aiRecsEnabled,
		}
	}

	return &domain.UserDataExport{
		ExportedAt:              now.Format(time.RFC3339),
		UserID:                  userID,
		ComplianceNotice:        "Compiled per GDPR Art. 15/20 & 16_security.md §9.1. Raw email content (Tier 4) is excluded per data minimization design (04_domain_model.md §6.3).",
		User:                    userMap,
		LinkedAccounts:          linkedAccounts,
		FinancialEvents:         events,
		EventOverrides:          overrides,
		Budgets:                 budgets,
		NotificationPreferences: preferences,
	}, nil
}
