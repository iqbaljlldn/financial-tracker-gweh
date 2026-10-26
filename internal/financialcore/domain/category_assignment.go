package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Category assignment sources per 04_domain_model.md §5.6.
const (
	CategorySourceUserHistory   = "user_history"
	CategorySourceGlobalMapping = "global_mapping"
	CategorySourceAIInference   = "ai_inference"
	CategorySourceManual        = "manual"
)

// CategoryAssignment is the entity recording the current effective categorization (04_domain_model.md §5.6).
// A later assignment supersedes the earlier one (kept for audit, mirroring EventOverride).
type CategoryAssignment struct {
	id           string
	category     string
	confidence   float64
	source       string
	assignedAt   time.Time
	supersededBy string
	isCurrent    bool
}

// CategoryAssignmentSnapshot carries persisted state for rehydration.
type CategoryAssignmentSnapshot struct {
	ID           string
	Category     string
	Confidence   float64
	Source       string
	AssignedAt   time.Time
	SupersededBy string
	IsCurrent    bool
}

func validCategorySource(source string) bool {
	switch source {
	case CategorySourceUserHistory, CategorySourceGlobalMapping, CategorySourceAIInference, CategorySourceManual:
		return true
	}
	return false
}

// NewCategoryAssignment creates a current assignment.
func NewCategoryAssignment(category string, confidence float64, source string, now time.Time) (*CategoryAssignment, error) {
	category = strings.TrimSpace(category)
	if category == "" || !validCategorySource(source) || confidence < 0 || confidence > 1 {
		return nil, fmt.Errorf("%w: category=%q source=%q confidence=%v", ErrInvalidCategory, category, source, confidence)
	}
	return &CategoryAssignment{
		id:         uuid.New().String(),
		category:   category,
		confidence: confidence,
		source:     source,
		assignedAt: now.UTC(),
		isCurrent:  true,
	}, nil
}

// RehydrateCategoryAssignment rebuilds an assignment from persisted state.
func RehydrateCategoryAssignment(s CategoryAssignmentSnapshot) *CategoryAssignment {
	return &CategoryAssignment{
		id:           s.ID,
		category:     s.Category,
		confidence:   s.Confidence,
		source:       s.Source,
		assignedAt:   s.AssignedAt,
		supersededBy: s.SupersededBy,
		isCurrent:    s.IsCurrent,
	}
}

func (c *CategoryAssignment) ID() string            { return c.id }
func (c *CategoryAssignment) Category() string      { return c.category }
func (c *CategoryAssignment) Confidence() float64   { return c.confidence }
func (c *CategoryAssignment) Source() string        { return c.source }
func (c *CategoryAssignment) AssignedAt() time.Time { return c.assignedAt }
func (c *CategoryAssignment) SupersededBy() string  { return c.supersededBy }
func (c *CategoryAssignment) IsCurrent() bool       { return c.isCurrent }

func (c *CategoryAssignment) supersede(byID string) {
	c.supersededBy = byID
	c.isCurrent = false
}
