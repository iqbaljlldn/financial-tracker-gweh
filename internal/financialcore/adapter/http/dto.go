package http

import (
	"encoding/json"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// AmountDTO carries monetary amounts as decimal strings, never JSON floats [GATE: 09_api_specification.md §6.2].
type AmountDTO struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// MerchantDTO represents merchant information.
type MerchantDTO struct {
	RawName        string `json:"rawName"`
	NormalizedName string `json:"normalizedName"`
}

// PaymentMethodDTO represents payment instrument hints.
type PaymentMethodDTO struct {
	Type string `json:"type"`
	Hint string `json:"hint"`
}

// CategoryDTO represents category assignment information.
type CategoryDTO struct {
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// ProvenanceDTO represents extraction metadata.
type ProvenanceDTO struct {
	ConnectorType    string `json:"connectorType"`
	ExtractionMethod string `json:"extractionMethod"`
	TemplateVersion  string `json:"templateVersion,omitempty"`
	PromptVersion    string `json:"promptVersion,omitempty"`
	ModelIdentifier  string `json:"modelIdentifier,omitempty"`
}

// FinancialEventResponse represents the public JSON projection of a FinancialEvent.
type FinancialEventResponse struct {
	ID                  string           `json:"id"`
	EventType           string           `json:"eventType"`
	Amount              AmountDTO        `json:"amount"`
	OccurredAt          string           `json:"occurredAt"`
	Merchant            MerchantDTO      `json:"merchant"`
	PaymentMethod       PaymentMethodDTO `json:"paymentMethod"`
	Status              string           `json:"status"`
	Confidence          float64          `json:"confidence"`
	Category            *CategoryDTO     `json:"category,omitempty"`
	Provenance          ProvenanceDTO    `json:"provenance"`
	TypeSpecificPayload map[string]any   `json:"typeSpecificPayload,omitempty"`
	CreatedAt           string           `json:"createdAt"`
	UpdatedAt           string           `json:"updatedAt"`
}

// PaginationDTO carries cursor-based pagination state.
type PaginationDTO struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// ListFinancialEventsResponse carries a paginated list of ledger events.
type ListFinancialEventsResponse struct {
	Items      []FinancialEventResponse `json:"items"`
	Pagination PaginationDTO            `json:"pagination"`
}

// CreateManualEventRequest carries input for POST /financial-events (FR-LEDGER-03).
type CreateManualEventRequest struct {
	EventType           string            `json:"eventType"`
	Amount              AmountDTO         `json:"amount"`
	OccurredAt          string            `json:"occurredAt"`
	Merchant            *MerchantDTO      `json:"merchant,omitempty"`
	PaymentMethod       *PaymentMethodDTO `json:"paymentMethod,omitempty"`
	Category            string            `json:"category,omitempty"`
	TypeSpecificPayload map[string]any    `json:"typeSpecificPayload,omitempty"`
}

// PatchFieldsDTO carries individual field override values.
type PatchFieldsDTO struct {
	Amount    *AmountDTO `json:"amount,omitempty"`
	Merchant  *string    `json:"merchant,omitempty"`
	EventType *string    `json:"eventType,omitempty"`
	Category  *string    `json:"category,omitempty"`
}

// PatchFinancialEventRequest carries input for PATCH /financial-events/{id}.
type PatchFinancialEventRequest struct {
	Fields PatchFieldsDTO `json:"fields"`
}

// ConfirmFinancialEventRequest carries input for POST /financial-events/{id}/confirm.
type ConfirmFinancialEventRequest struct {
	Confirmed   *bool           `json:"confirmed,omitempty"`
	Corrections *PatchFieldsDTO `json:"corrections,omitempty"`
}

// EventOverrideResponse represents a manual correction record in the audit trail.
type EventOverrideResponse struct {
	ID               string          `json:"id"`
	FinancialEventID string          `json:"financialEventId"`
	FieldName        string          `json:"fieldName"`
	PreviousValue    json.RawMessage `json:"previousValue"`
	NewValue         json.RawMessage `json:"newValue"`
	ActorUserID      string          `json:"actorUserId"`
	CorrectedAt      string          `json:"correctedAt"`
}

// ErrorEnvelope matches 09_api_specification.md §7.2.
type ErrorEnvelope struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails carries structured error metadata.
type ErrorDetails struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// ToEventResponse converts a domain FinancialEvent to the API response DTO.
// Notice that effective values are surfaced for amount, merchant, and eventType.
func ToEventResponse(e *domain.FinancialEvent, includeTypeSpecific bool) FinancialEventResponse {
	effAmount := e.EffectiveAmount()
	effMerchant := e.EffectiveMerchant()
	prov := e.Provenance()

	resp := FinancialEventResponse{
		ID:        e.ID(),
		EventType: string(e.EffectiveEventType()),
		Amount: AmountDTO{
			Value:    effAmount.Value(),
			Currency: effAmount.Currency(),
		},
		OccurredAt: e.OccurredAt().Format(time.RFC3339),
		Merchant: MerchantDTO{
			RawName:        effMerchant.RawName,
			NormalizedName: effMerchant.NormalizedName,
		},
		PaymentMethod: PaymentMethodDTO{
			Type: e.PaymentMethod().Type,
			Hint: e.PaymentMethod().Hint,
		},
		Status:     string(e.Status()),
		Confidence: e.Confidence().Aggregate,
		Provenance: ProvenanceDTO{
			ConnectorType:    prov.ConnectorType(),
			ExtractionMethod: string(prov.ExtractionMethod()),
			TemplateVersion:  prov.TemplateVersion(),
			PromptVersion:    prov.PromptVersion(),
			ModelIdentifier:  prov.ModelIdentifier(),
		},
		CreatedAt: e.CreatedAt().Format(time.RFC3339),
		UpdatedAt: e.UpdatedAt().Format(time.RFC3339),
	}

	if cat := e.Category(); cat != nil {
		resp.Category = &CategoryDTO{
			Value:      cat.Category(),
			Source:     cat.Source(),
			Confidence: cat.Confidence(),
		}
	}

	if includeTypeSpecific {
		resp.TypeSpecificPayload = e.TypeSpecificPayload()
	}

	return resp
}

// ToOverrideResponse converts a domain EventOverride to the API response DTO.
func ToOverrideResponse(o *domain.EventOverride) EventOverrideResponse {
	return EventOverrideResponse{
		ID:               o.ID(),
		FinancialEventID: o.FinancialEventID(),
		FieldName:        o.FieldName(),
		PreviousValue:    o.PreviousValue(),
		NewValue:         o.NewValue(),
		ActorUserID:      o.ActorUserID(),
		CorrectedAt:      o.CorrectedAt().Format(time.RFC3339),
	}
}
