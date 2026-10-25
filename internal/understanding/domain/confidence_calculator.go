package domain

import "math"

// ConfidenceWeights configures the relative weight of each extractable field in aggregate scoring.
// Stored as reviewable, version-controlled configuration per 11_ai_blueprint.md §6.2 & §18 [GATE].
type ConfidenceWeights struct {
	Amount        float64 `json:"amount" yaml:"amount"`
	Merchant      float64 `json:"merchant" yaml:"merchant"`
	EventType     float64 `json:"eventType" yaml:"event_type"`
	OccurredAt    float64 `json:"occurredAt" yaml:"occurred_at"`
	PaymentMethod float64 `json:"paymentMethod" yaml:"payment_method"`
	CategoryHint  float64 `json:"categoryHint" yaml:"category_hint"`
}

// DefaultConfidenceWeights returns the baseline weights specified in 11_ai_blueprint.md §6.2.
func DefaultConfidenceWeights() ConfidenceWeights {
	return ConfidenceWeights{
		Amount:        0.35,
		Merchant:      0.20,
		EventType:     0.20,
		OccurredAt:    0.15,
		PaymentMethod: 0.05,
		CategoryHint:  0.05,
	}
}

// FieldConfidences captures model self-reported confidence per extracted field.
type FieldConfidences struct {
	Amount        *float64
	Merchant      *float64
	EventType     *float64
	OccurredAt    *float64
	PaymentMethod *float64
	CategoryHint  *float64
}

// ComputeAggregateConfidence calculates deterministic, auditable weighted aggregate confidence (11_ai_blueprint.md §6.2).
// Missing or null fields contribute 0.0 confidence in the weighted sum.
func ComputeAggregateConfidence(fc FieldConfidences, weights ConfidenceWeights) (float64, map[string]float64) {
	perField := make(map[string]float64)

	getConf := func(val *float64) float64 {
		if val == nil {
			return 0.0
		}
		c := *val
		if c < 0.0 {
			c = 0.0
		} else if c > 1.0 {
			c = 1.0
		}
		return c
	}

	amtConf := getConf(fc.Amount)
	merchConf := getConf(fc.Merchant)
	eventConf := getConf(fc.EventType)
	occConf := getConf(fc.OccurredAt)
	pmConf := getConf(fc.PaymentMethod)
	catConf := getConf(fc.CategoryHint)

	perField["amount"] = amtConf
	perField["merchant"] = merchConf
	perField["eventType"] = eventConf
	perField["occurredAt"] = occConf
	perField["paymentMethod"] = pmConf
	perField["categoryHint"] = catConf

	aggregate := (amtConf * weights.Amount) +
		(merchConf * weights.Merchant) +
		(eventConf * weights.EventType) +
		(occConf * weights.OccurredAt) +
		(pmConf * weights.PaymentMethod) +
		(catConf * weights.CategoryHint)

	// Round to 3 decimal places for precision matching NUMERIC(4,3)
	rounded := math.Round(aggregate*1000) / 1000
	if rounded < 0.0 {
		rounded = 0.0
	} else if rounded > 1.0 {
		rounded = 1.0
	}

	return rounded, perField
}
