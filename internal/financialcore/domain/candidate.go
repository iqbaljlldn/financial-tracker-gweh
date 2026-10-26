package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Candidate is financialcore's anti-corruption view of the CandidateFinancialEvent contract
// (04_domain_model.md §7.2) produced by the understanding module. Module boundaries forbid importing
// understanding's Go types (06_architecture.md §5.2), so the contract is shared as JSON shape only.
type Candidate struct {
	UserID              string              `json:"userId"`
	EventType           string              `json:"eventType"`
	Amount              CandidateAmount     `json:"amount"`
	OccurredAt          time.Time           `json:"occurredAt"`
	Merchant            Merchant            `json:"merchant"`
	PaymentMethod       CandidatePayment    `json:"paymentMethod"`
	Confidence          *Confidence         `json:"confidence"`
	Provenance          CandidateProvenance `json:"provenance"`
	TypeSpecificPayload map[string]any      `json:"typeSpecificPayload,omitempty"`
}

// CandidateAmount mirrors the extractor's amount (a float64 value at this single boundary).
type CandidateAmount struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// CandidatePayment mirrors the extractor's payment method.
type CandidatePayment struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	Last4  string `json:"last4"`
}

// CandidateProvenance mirrors the extractor's provenance.
type CandidateProvenance struct {
	ConnectorType      string    `json:"connectorType"`
	ConnectorAccountID string    `json:"connectorAccountId"`
	SourceMessageRef   string    `json:"sourceMessageRef"`
	ExtractionMethod   string    `json:"extractionMethod"`
	TemplateVersion    string    `json:"templateVersion,omitempty"`
	PromptVersion      string    `json:"promptVersion,omitempty"`
	ModelIdentifier    string    `json:"modelIdentifier,omitempty"`
	ExtractedAt        time.Time `json:"extractedAt"`
}

// CandidateCreatedPayload is the FinancialEventCandidateCreated outbox payload consumed by the gate.
type CandidateCreatedPayload struct {
	ExtractionResultID string    `json:"extractionResultId"`
	IngestedMessageID  string    `json:"ingestedMessageId"`
	Candidate          Candidate `json:"candidate"`
	OccurredAt         time.Time `json:"occurredAt"`
}

// Reserved typeSpecificPayload keys the extractor uses to carry its candidate-time category suggestion.
const (
	payloadKeyCategory           = "category"
	payloadKeyCategorySource     = "categorySource"
	payloadKeyCategoryConfidence = "categoryConfidence"
	// UncategorizedLabel is the extractor's fallback tier; it is not persisted as an assignment.
	UncategorizedLabel = "Uncategorized"
)

// CategorySuggestion is a candidate-time category suggestion (H5 resolution).
type CategorySuggestion struct {
	Category   string
	Confidence float64
	Source     string
}

// ParseCandidatePayload decodes a raw FinancialEventCandidateCreated payload.
func ParseCandidatePayload(raw []byte) (*CandidateCreatedPayload, error) {
	var p CandidateCreatedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: decoding candidate payload: %v", ErrInvalidCandidate, err)
	}
	return &p, nil
}

// SuggestedCategory extracts the extractor's candidate-time category suggestion, if it carries a usable one.
// The "default"/Uncategorized fallback tier is not a persisted assignment (it has no allowed source in
// 08_database_design.md §7.3), so it is reported as absent and rendered as "Uncategorized" by readers.
func (c *Candidate) SuggestedCategory() (CategorySuggestion, bool) {
	cat, _ := c.TypeSpecificPayload[payloadKeyCategory].(string)
	cat = strings.TrimSpace(cat)
	if cat == "" || strings.EqualFold(cat, UncategorizedLabel) {
		return CategorySuggestion{}, false
	}
	source, _ := c.TypeSpecificPayload[payloadKeyCategorySource].(string)
	if !validCategorySource(source) {
		return CategorySuggestion{}, false
	}
	conf, _ := c.TypeSpecificPayload[payloadKeyCategoryConfidence].(float64)
	if conf < 0 || conf > 1 {
		conf = 0
	}
	return CategorySuggestion{Category: cat, Confidence: conf, Source: source}, true
}

// attributesWithoutCategory returns the type-specific attributes minus the reserved category keys,
// which are persisted as a CategoryAssignment rather than inside the payload.
func (c *Candidate) attributesWithoutCategory() map[string]any {
	out := make(map[string]any, len(c.TypeSpecificPayload))
	for k, v := range c.TypeSpecificPayload {
		switch k {
		case payloadKeyCategory, payloadKeyCategorySource, payloadKeyCategoryConfidence:
			continue
		}
		out[k] = v
	}
	return out
}
