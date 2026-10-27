package domain

import (
	"time"
)

// CashFlowBucket represents a time-bucketed aggregation of income vs expenses per currency.
type CashFlowBucket struct {
	PeriodBucket string    `json:"periodBucket"` // "YYYY-MM-DD"
	Granularity  string    `json:"granularity"`  // "daily" | "monthly"
	Currency     string    `json:"currency"`     // ISO 4217 code
	TotalIncome  string    `json:"totalIncome"`  // Decimal string (e.g. "5200.00")
	TotalExpense string    `json:"totalExpense"` // Decimal string (e.g. "3180.45")
	UpdatedAt    time.Time `json:"updatedAt"`
}

// CategoryBreakdownItem represents the spending breakdown per category per currency.
type CategoryBreakdownItem struct {
	PeriodBucket    string    `json:"periodBucket"`    // "YYYY-MM-DD"
	Category        string    `json:"category"`        // Category name
	Currency        string    `json:"currency"`        // ISO 4217 code
	TotalValue      string    `json:"totalValue"`      // Decimal string
	PercentageShare float64   `json:"percentageShare"` // e.g. 42.50
	UpdatedAt       time.Time `json:"updatedAt"`
}

// MerchantRankingItem represents the spending rank and count per merchant per currency.
type MerchantRankingItem struct {
	PeriodBucket           string    `json:"periodBucket"`           // "YYYY-MM-DD"
	MerchantNormalizedName string    `json:"merchantNormalizedName"` // Merchant name
	Currency               string    `json:"currency"`               // ISO 4217 code
	TotalValue             string    `json:"totalValue"`             // Decimal string
	TransactionCount       int32     `json:"transactionCount"`       // Number of transactions
	UpdatedAt              time.Time `json:"updatedAt"`
}

// TrendItem represents the net cash flow (income - expense) time series per currency.
type TrendItem struct {
	PeriodBucket string    `json:"periodBucket"` // "YYYY-MM-DD"
	Granularity  string    `json:"granularity"`  // "daily" | "monthly"
	Currency     string    `json:"currency"`     // ISO 4217 code
	NetFlow      string    `json:"netFlow"`      // Decimal string (can be negative)
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ProjectionCheckpoint tracks the last successfully projected outbox event per source module.
type ProjectionCheckpoint struct {
	SourceModule               string    `json:"sourceModule"`
	LastProcessedOutboxEventID string    `json:"lastProcessedOutboxEventId"`
	CheckpointAt               time.Time `json:"checkpointAt"`
}

// ProjectedEvent tracks the state of each projected financial event in the analytics schema.
type ProjectedEvent struct {
	EventID                string
	UserID                 string
	EventType              string
	AmountValue            float64
	Currency               string
	OccurredAt             time.Time
	MerchantNormalizedName string
	Category               string
	Status                 string // "active" | "archived"
	UpdatedAt              time.Time
}
