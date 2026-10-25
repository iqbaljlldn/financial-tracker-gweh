package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidAmount     = errors.New("candidate financial event: amount value must be positive")
	ErrMissingCurrency   = errors.New("candidate financial event: currency is required")
	ErrMissingOccurredAt = errors.New("candidate financial event: occurredAt is required")
	ErrMissingEventType  = errors.New("candidate financial event: eventType is required")
	ErrInvalidConfidence = errors.New("candidate financial event: confidence aggregate must be between 0.0 and 1.0")
)

// Supported event types per 04_domain_model.md §5.2
const (
	EventTypeIncome              = "Income"
	EventTypeSalary              = "Salary"
	EventTypeDividend            = "Dividend"
	EventTypeExpense             = "Expense"
	EventTypeTransfer            = "Transfer"
	EventTypeRefund              = "Refund"
	EventTypeCashback            = "Cashback"
	EventTypeSubscriptionRenewal = "SubscriptionRenewal"
	EventTypeFailedPayment       = "FailedPayment"
	EventTypeInvestment          = "Investment"
	EventTypeLoan                = "Loan"
	EventTypeCreditCardBill      = "CreditCardBill"
	EventTypeTax                 = "Tax"
	EventTypeDonation            = "Donation"
	EventTypeOthers              = "Others"
)

// ExtractionMethod values per 04_domain_model.md §5.4
const (
	ExtractionMethodTemplate = "template"
	ExtractionMethodAI       = "ai"
	ExtractionMethodManual   = "manual"
)

// Money value object per 04_domain_model.md §5.1.1
type Money struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// Merchant value object per 04_domain_model.md §5.1.1
type Merchant struct {
	NormalizedName string  `json:"normalizedName"`
	RawName        string  `json:"rawName"`
	MerchantID     *string `json:"merchantId,omitempty"`
}

// PaymentMethod value object per 04_domain_model.md §5.1.1
type PaymentMethod struct {
	Type   string `json:"type"`   // e.g., "credit_card", "e_wallet", "bank_transfer"
	Detail string `json:"detail"` // e.g., "BCA OneKlik", "GoPay Balance", "Visa"
	Last4  string `json:"last4"`  // e.g., "4321" or empty
}

// Confidence value object per 04_domain_model.md §5.1.1 & §7.2
type Confidence struct {
	Aggregate float64            `json:"aggregate"`
	PerField  map[string]float64 `json:"perField,omitempty"`
}

// Provenance value object per 04_domain_model.md §5.4
type Provenance struct {
	ConnectorType      string    `json:"connectorType"`
	ConnectorAccountID string    `json:"connectorAccountId"`
	SourceMessageRef   string    `json:"sourceMessageRef"`
	ExtractionMethod   string    `json:"extractionMethod"` // "template" | "ai"
	TemplateVersion    string    `json:"templateVersion,omitempty"`
	PromptVersion      string    `json:"promptVersion,omitempty"`
	ModelIdentifier    string    `json:"modelIdentifier,omitempty"`
	ExtractedAt        time.Time `json:"extractedAt"`
}

// CandidateFinancialEvent value object per 04_domain_model.md §7.2.
// Its shape exactly mirrors the FinancialEvent common envelope plus type-specific payload.
// [GATE]: Coordination with Sprint 6's schema.
type CandidateFinancialEvent struct {
	UserID              string         `json:"userId"`
	EventType           string         `json:"eventType"`
	Amount              Money          `json:"amount"`
	OccurredAt          time.Time      `json:"occurredAt"`
	Merchant            Merchant       `json:"merchant"`
	PaymentMethod       PaymentMethod  `json:"paymentMethod"`
	Confidence          Confidence     `json:"confidence"`
	Provenance          Provenance     `json:"provenance"`
	TypeSpecificPayload map[string]any `json:"typeSpecificPayload,omitempty"`
}

// Validate verifies candidate event invariants.
func (c *CandidateFinancialEvent) Validate() error {
	if c.EventType == "" {
		return ErrMissingEventType
	}
	if c.Amount.Value <= 0 {
		return fmt.Errorf("%w: got %f", ErrInvalidAmount, c.Amount.Value)
	}
	if strings.TrimSpace(c.Amount.Currency) == "" {
		return ErrMissingCurrency
	}
	if c.OccurredAt.IsZero() {
		return ErrMissingOccurredAt
	}
	if c.Confidence.Aggregate < 0.0 || c.Confidence.Aggregate > 1.0 {
		return fmt.Errorf("%w: got %f", ErrInvalidConfidence, c.Confidence.Aggregate)
	}
	return nil
}
