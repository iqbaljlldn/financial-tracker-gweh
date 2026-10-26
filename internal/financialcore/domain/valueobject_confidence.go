package domain

import (
	"fmt"
	"math"
)

// ConfidenceMethod enumerates how the aggregate confidence was produced (08_database_design.md §7.1).
type ConfidenceMethod string

const (
	ConfidenceMethodTemplate ConfidenceMethod = "template_deterministic"
	ConfidenceMethodAI       ConfidenceMethod = "ai_weighted"
	ConfidenceMethodManual   ConfidenceMethod = "manual"
)

// DefaultConfidenceThreshold is the gate threshold used when none is configured (07_sequence_diagrams.md Diagram 3).
const DefaultConfidenceThreshold = 0.70

// Confidence is the confidence value object (04_domain_model.md §5.1.1 & §7.2). Its JSON shape is
// intentionally identical to the understanding module's candidate confidence so both sides of the
// FinancialEventCandidateCreated contract agree without sharing Go types across module boundaries.
type Confidence struct {
	Aggregate float64            `json:"aggregate"`
	PerField  map[string]float64 `json:"perField,omitempty"`
}

// GateDecision is the recorded outcome of evaluating a Confidence against the threshold.
// A zero-value GateDecision (Evaluated == false) can never finalize or create an event (INV-1).
type GateDecision struct {
	Evaluated bool
	Aggregate float64
	Threshold float64
	Passed    bool
}

// EvaluateConfidenceGate evaluates confidence against the current threshold.
// It enforces the evaluation half of INV-1 (04_domain_model.md §5.7): an absent, NaN or out-of-range
// confidence is never "evaluated", so no event can reach persistence without passing through here.
func EvaluateConfidenceGate(c *Confidence, threshold float64) (GateDecision, error) {
	if c == nil || math.IsNaN(c.Aggregate) || c.Aggregate < 0 || c.Aggregate > 1 {
		return GateDecision{}, ErrConfidenceNotEvaluated
	}
	if threshold <= 0 || threshold > 1 {
		return GateDecision{}, fmt.Errorf("%w: threshold %v is outside (0,1]", ErrConfidenceNotEvaluated, threshold)
	}
	return GateDecision{
		Evaluated: true,
		Aggregate: c.Aggregate,
		Threshold: threshold,
		Passed:    c.Aggregate >= threshold,
	}, nil
}

// MethodForExtraction maps a provenance extraction method onto its confidence method.
func MethodForExtraction(extraction ExtractionMethod) ConfidenceMethod {
	switch extraction {
	case ExtractionMethodTemplate:
		return ConfidenceMethodTemplate
	case ExtractionMethodAI:
		return ConfidenceMethodAI
	default:
		return ConfidenceMethodManual
	}
}
