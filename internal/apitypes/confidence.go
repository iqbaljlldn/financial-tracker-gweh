// Package apitypes provides shared, minimal wire-format value-object shapes.
package apitypes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

var (
	// ErrInvalidConfidenceScore indicates a confidence score outside the [0.0, 1.0] interval.
	ErrInvalidConfidenceScore = errors.New("invalid confidence score: must be between 0.0 and 1.0")
	// ErrInvalidConfidencePayload indicates malformed JSON when decoding a Confidence value object.
	ErrInvalidConfidencePayload = errors.New("invalid confidence payload: expected number or object")
)

// Confidence represents an evaluated extraction certainty score per 04_domain_model.md §14.
// It supports both aggregate confidence scoring (used for INV-1 gating)
// and rich contextual metadata (computation method, per-field scores) for review queues.
type Confidence struct {
	Value    float64            `json:"value"`
	Method   string             `json:"method,omitempty"`
	PerField map[string]float64 `json:"perField,omitempty"`
}

// NewConfidence creates a validated simple Confidence score.
func NewConfidence(score float64) (Confidence, error) {
	c := Confidence{Value: score}
	if err := c.Validate(); err != nil {
		return Confidence{}, err
	}
	return c, nil
}

// NewRichConfidence creates a validated Confidence score with method tag and optional per-field scores.
func NewRichConfidence(score float64, method string, perField map[string]float64) (Confidence, error) {
	c := Confidence{
		Value:    score,
		Method:   method,
		PerField: perField,
	}
	if err := c.Validate(); err != nil {
		return Confidence{}, err
	}
	return c, nil
}

// Validate ensures the aggregate and any per-field confidence scores fall in [0.0, 1.0].
func (c Confidence) Validate() error {
	if math.IsNaN(c.Value) || math.IsInf(c.Value, 0) || c.Value < 0.0 || c.Value > 1.0 {
		return fmt.Errorf("%w: aggregate score %v", ErrInvalidConfidenceScore, c.Value)
	}
	for field, score := range c.PerField {
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0.0 || score > 1.0 {
			return fmt.Errorf("%w: field %q has score %v", ErrInvalidConfidenceScore, field, score)
		}
	}
	return nil
}

// MarshalJSON marshals Confidence as a bare float if no extra metadata is present,
// or as a structured object if method or perField metadata is provided.
func (c Confidence) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Method == "" && len(c.PerField) == 0 {
		return json.Marshal(c.Value)
	}
	type alias Confidence
	return json.Marshal(alias(c))
}

// UnmarshalJSON unmarshals either a bare float or a structured object into Confidence.
func (c *Confidence) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ErrInvalidConfidencePayload
	}

	// Try unmarshaling as float first
	var score float64
	if err := json.Unmarshal(data, &score); err == nil {
		c.Value = score
		c.Method = ""
		c.PerField = nil
		return c.Validate()
	}

	// Otherwise try unmarshaling as structured object
	type alias struct {
		Value    float64            `json:"value"`
		Score    *float64           `json:"score,omitempty"`
		Method   string             `json:"method,omitempty"`
		PerField map[string]float64 `json:"perField,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfidencePayload, err)
	}

	if a.Score != nil && a.Value == 0 {
		c.Value = *a.Score
	} else {
		c.Value = a.Value
	}
	c.Method = a.Method
	c.PerField = a.PerField

	return c.Validate()
}
