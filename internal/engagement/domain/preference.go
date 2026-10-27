package domain

import (
	"github.com/google/uuid"
)

// NotificationPreferences represents per-user notification channel preferences.
// Per FR-NOTIF-05: opting out of one notification_type must never affect others.
type NotificationPreferences struct {
	UserID                   uuid.UUID
	PhoneNumber              string
	DailySummaryEnabled      bool
	WeeklySummaryEnabled     bool
	MonthlyReportEnabled     bool
	BudgetAlertsEnabled      bool
	AiRecommendationsEnabled bool
}

// DefaultPreferences returns safe defaults: standard summaries and alerts enabled.
func DefaultPreferences(userID uuid.UUID) *NotificationPreferences {
	return &NotificationPreferences{
		UserID:                   userID,
		DailySummaryEnabled:      true,
		WeeklySummaryEnabled:     true,
		MonthlyReportEnabled:     true,
		BudgetAlertsEnabled:      true,
		AiRecommendationsEnabled: false,
	}
}

// IsEnabled checks if a specific notification type is allowed.
// Opted-out types result in a clean no-op at the Preference Gate.
func (p *NotificationPreferences) IsEnabled(t NotificationType) bool {
	if p == nil {
		return false
	}
	switch t {
	case NotificationTypeDailySummary:
		return p.DailySummaryEnabled
	case NotificationTypeWeeklySummary:
		return p.WeeklySummaryEnabled
	case NotificationTypeMonthlyReport:
		return p.MonthlyReportEnabled
	case NotificationTypeBudgetAlert:
		return p.BudgetAlertsEnabled
	case NotificationTypeAiRecommendation:
		return p.AiRecommendationsEnabled
	case NotificationTypeReviewRequest:
		// Review requests are critical safety interactions for pending-review ledger items;
		// they remain enabled as long as a phone number is registered.
		return p.PhoneNumber != ""
	default:
		return false
	}
}
