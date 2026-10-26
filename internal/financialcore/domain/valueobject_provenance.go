package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExtractionMethod values per 04_domain_model.md §5.4.
type ExtractionMethod string

const (
	ExtractionMethodTemplate ExtractionMethod = "template"
	ExtractionMethodAI       ExtractionMethod = "ai"
	ExtractionMethodManual   ExtractionMethod = "manual"
)

// Provenance is the immutable lineage value object (04_domain_model.md §5.4).
// All fields are unexported and there are no setters: once built by NewProvenance it can only be read,
// which is how "Provenance is immutable once set" is enforced at compile time. Corrections are
// recorded as EventOverrides, never by mutating Provenance.
type Provenance struct {
	connectorType      string
	connectorAccountID string
	sourceMessageRef   string
	extractionMethod   ExtractionMethod
	templateVersion    string
	promptVersion      string
	modelIdentifier    string
	extractedAt        time.Time
}

// ProvenanceParams carries the inputs for NewProvenance.
type ProvenanceParams struct {
	ConnectorType      string
	ConnectorAccountID string
	SourceMessageRef   string
	ExtractionMethod   ExtractionMethod
	TemplateVersion    string
	PromptVersion      string
	ModelIdentifier    string
	ExtractedAt        time.Time
}

// NewProvenance validates and freezes provenance.
func NewProvenance(p ProvenanceParams) (Provenance, error) {
	if strings.TrimSpace(p.ConnectorType) == "" {
		return Provenance{}, fmt.Errorf("%w: provenance.connectorType is required", ErrInvalidCandidate)
	}
	if _, err := uuid.Parse(p.ConnectorAccountID); err != nil {
		return Provenance{}, fmt.Errorf("%w: provenance.connectorAccountId must be a UUID", ErrInvalidCandidate)
	}
	if strings.TrimSpace(p.SourceMessageRef) == "" {
		return Provenance{}, fmt.Errorf("%w: provenance.sourceMessageRef is required", ErrInvalidCandidate)
	}
	switch p.ExtractionMethod {
	case ExtractionMethodTemplate, ExtractionMethodAI, ExtractionMethodManual:
	default:
		return Provenance{}, fmt.Errorf("%w: unsupported extraction method %q", ErrInvalidCandidate, p.ExtractionMethod)
	}
	if p.ExtractedAt.IsZero() {
		return Provenance{}, fmt.Errorf("%w: provenance.extractedAt is required", ErrInvalidCandidate)
	}

	return Provenance{
		connectorType:      p.ConnectorType,
		connectorAccountID: p.ConnectorAccountID,
		sourceMessageRef:   p.SourceMessageRef,
		extractionMethod:   p.ExtractionMethod,
		templateVersion:    p.TemplateVersion,
		promptVersion:      p.PromptVersion,
		modelIdentifier:    p.ModelIdentifier,
		extractedAt:        p.ExtractedAt.UTC(),
	}, nil
}

func (p Provenance) ConnectorType() string              { return p.connectorType }
func (p Provenance) ConnectorAccountID() string         { return p.connectorAccountID }
func (p Provenance) SourceMessageRef() string           { return p.sourceMessageRef }
func (p Provenance) ExtractionMethod() ExtractionMethod { return p.extractionMethod }
func (p Provenance) TemplateVersion() string            { return p.templateVersion }
func (p Provenance) PromptVersion() string              { return p.promptVersion }
func (p Provenance) ModelIdentifier() string            { return p.modelIdentifier }
func (p Provenance) ExtractedAt() time.Time             { return p.extractedAt }

// Params returns the provenance inputs so adapters can persist or rehydrate it.
func (p Provenance) Params() ProvenanceParams {
	return ProvenanceParams{
		ConnectorType:      p.connectorType,
		ConnectorAccountID: p.connectorAccountID,
		SourceMessageRef:   p.sourceMessageRef,
		ExtractionMethod:   p.extractionMethod,
		TemplateVersion:    p.templateVersion,
		PromptVersion:      p.promptVersion,
		ModelIdentifier:    p.modelIdentifier,
		ExtractedAt:        p.extractedAt,
	}
}
