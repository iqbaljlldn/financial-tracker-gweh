package domain

import "context"

// UserDataExport encapsulates the complete, portable user profile and financial history
// for GDPR Article 15 (Right of Access) / Article 20 (Data Portability) per 16_security.md §9.1.
// Raw email content (Tier 4) is explicitly excluded per minimization policy.
type UserDataExport struct {
	ExportedAt              string `json:"exportedAt"`
	UserID                  string `json:"userId"`
	ComplianceNotice        string `json:"complianceNotice"`
	User                    any    `json:"user"`
	LinkedAccounts          any    `json:"linkedAccounts"`
	FinancialEvents         any    `json:"financialEvents"`
	EventOverrides          any    `json:"eventOverrides"`
	Budgets                 any    `json:"budgets"`
	NotificationPreferences any    `json:"notificationPreferences"`
}

// UserDataExporter compiles the user's data archive.
type UserDataExporter interface {
	Export(ctx context.Context, userID string) (*UserDataExport, error)
}
