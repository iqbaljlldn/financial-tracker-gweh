package query

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

type CategoryBreakdownResult struct {
	Items               []domain.CategoryBreakdownItem `json:"items"`
	ProjectionFreshness *time.Time                     `json:"projectionFreshness"`
}

type GetCategoryBreakdownHandler struct {
	repo domain.Repository
}

func NewGetCategoryBreakdownHandler(repo domain.Repository) *GetCategoryBreakdownHandler {
	return &GetCategoryBreakdownHandler{repo: repo}
}

type GetCategoryBreakdownParams struct {
	UserID   string
	From     *time.Time
	To       *time.Time
	Currency *string
}

func (h *GetCategoryBreakdownHandler) Handle(ctx context.Context, params GetCategoryBreakdownParams) (*CategoryBreakdownResult, error) {
	items, err := h.repo.GetCategoryBreakdown(ctx, params.UserID, params.From, params.To, params.Currency)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []domain.CategoryBreakdownItem{}
	}

	cp, _ := h.repo.GetProjectionCheckpoint(ctx, "financialcore")
	var freshness *time.Time
	if cp != nil && !cp.CheckpointAt.IsZero() {
		freshness = &cp.CheckpointAt
	}

	return &CategoryBreakdownResult{
		Items:               items,
		ProjectionFreshness: freshness,
	}, nil
}
