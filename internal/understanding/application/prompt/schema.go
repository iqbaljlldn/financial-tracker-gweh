package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

var (
	ErrInvalidJSON       = errors.New("raw response is not valid JSON")
	ErrMissingEventType  = errors.New("schema validation: eventType is required")
	ErrInvalidEventType  = errors.New("schema validation: unrecognized eventType")
	ErrInvalidAmount     = errors.New("schema validation: amount value must be a positive number")
	ErrMissingCurrency   = errors.New("schema validation: currency must be a 3-letter code")
	ErrInvalidOccurredAt = errors.New("schema validation: occurredAt must be a valid RFC3339 timestamp")
)

var codeFenceRegex = regexp.MustCompile(`(?s)^` + "```" + `(?:json)?\s*([\s\S]*?)\s*` + "```" + `$`)

// RawAIExtractionOutput defines the structured JSON contract produced by LLM extraction (11_ai_blueprint.md §5).
type RawAIExtractionOutput struct {
	IsFinancialTransaction *bool  `json:"isFinancialTransaction,omitempty"`
	EventType              string `json:"eventType"`
	Amount                 struct {
		Value      any     `json:"value"` // can be string ("45.00") or float64 (45.00)
		Currency   string  `json:"currency"`
		Confidence float64 `json:"confidence"`
	} `json:"amount"`
	OccurredAt struct {
		Value      string  `json:"value"` // ISO8601/RFC3339 e.g. "2026-08-03T14:22:00Z"
		Confidence float64 `json:"confidence"`
	} `json:"occurredAt"`
	Merchant struct {
		RawName        string  `json:"rawName"`
		NormalizedName string  `json:"normalizedName"`
		Confidence     float64 `json:"confidence"`
	} `json:"merchant"`
	PaymentMethod struct {
		Type       string  `json:"type"` // e.g. "card", "e_wallet", "bank_transfer"
		Hint       string  `json:"hint"` // e.g. "4821"
		Confidence float64 `json:"confidence"`
	} `json:"paymentMethod"`
	CategoryHint struct {
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
	} `json:"categoryHint"`
	TypeSpecificPayload map[string]any `json:"typeSpecificPayload,omitempty"`
	ExtractionNotes     string         `json:"extractionNotes,omitempty"`
}

// CleanJSONContent strips any markdown code fences surrounding JSON output.
func CleanJSONContent(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if match := codeFenceRegex.FindStringSubmatch(trimmed); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return trimmed
}

// ValidateRawOutput enforces the schema matching CandidateFinancialEvent envelope (11_ai_blueprint.md §5).
func ValidateRawOutput(rawJSON string) (*RawAIExtractionOutput, error) {
	cleaned := CleanJSONContent(rawJSON)
	var output RawAIExtractionOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	// Non-transaction email recognized by LLM
	if output.IsFinancialTransaction != nil && !*output.IsFinancialTransaction {
		return &output, nil
	}
	if strings.EqualFold(output.EventType, "None") || strings.EqualFold(output.EventType, "NonTransaction") {
		f := false
		output.IsFinancialTransaction = &f
		return &output, nil
	}

	if strings.TrimSpace(output.EventType) == "" {
		return nil, ErrMissingEventType
	}

	// Validate amount
	amtVal, err := ParseAmountValue(output.Amount.Value)
	if err != nil || amtVal <= 0 {
		return nil, ErrInvalidAmount
	}

	curr := strings.ToUpper(strings.TrimSpace(output.Amount.Currency))
	if len(curr) != 3 {
		return nil, ErrMissingCurrency
	}

	// Validate occurredAt timestamp
	if strings.TrimSpace(output.OccurredAt.Value) == "" {
		return nil, ErrInvalidOccurredAt
	}
	if _, err := parseOccurredAtTime(output.OccurredAt.Value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOccurredAt, err)
	}

	return &output, nil
}

// ParseAmountValue parses numeric amount from string or float.
func ParseAmountValue(val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		cleaned := strings.TrimSpace(v)
		cleaned = strings.ReplaceAll(cleaned, ",", "")
		return strconv.ParseFloat(cleaned, 64)
	default:
		return 0, fmt.Errorf("unsupported amount value type: %T", val)
	}
}

func parseOccurredAtTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("unable to parse timestamp format")
}

// ConvertToDomainCandidate maps RawAIExtractionOutput to domain.CandidateFinancialEvent.
func ConvertToDomainCandidate(
	out *RawAIExtractionOutput,
	userID string,
	provenance domain.Provenance,
	confidenceAggregate float64,
	confidencePerField map[string]float64,
) (*domain.CandidateFinancialEvent, error) {
	amtVal, _ := ParseAmountValue(out.Amount.Value)
	occurredAt, _ := parseOccurredAtTime(out.OccurredAt.Value)

	cand := &domain.CandidateFinancialEvent{
		UserID:    userID,
		EventType: out.EventType,
		Amount: domain.Money{
			Value:    amtVal,
			Currency: strings.ToUpper(strings.TrimSpace(out.Amount.Currency)),
		},
		OccurredAt: occurredAt,
		Merchant: domain.Merchant{
			RawName:        out.Merchant.RawName,
			NormalizedName: out.Merchant.NormalizedName,
		},
		PaymentMethod: domain.PaymentMethod{
			Type:   out.PaymentMethod.Type,
			Detail: out.PaymentMethod.Type,
			Last4:  out.PaymentMethod.Hint,
		},
		Confidence: domain.Confidence{
			Aggregate: confidenceAggregate,
			PerField:  confidencePerField,
		},
		Provenance:          provenance,
		TypeSpecificPayload: out.TypeSpecificPayload,
	}

	return cand, nil
}
