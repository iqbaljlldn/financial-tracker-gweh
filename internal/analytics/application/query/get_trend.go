package query

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

type TrendResult struct {
	Items               []domain.TrendItem `json:"items"`
	ProjectionFreshness *time.Time         `json:"projectionFreshness"`
}

type GetTrendHandler struct {
	repo domain.Repository
}

func NewGetTrendHandler(repo domain.Repository) *GetTrendHandler {
	return &GetTrendHandler{repo: repo}
}

type GetTrendParams struct {
	UserID      string
	Granularity string
	From        *time.Time
	To          *time.Time
	Currency    *string
}

func (h *GetTrendHandler) Handle(ctx context.Context, params GetTrendParams) (*TrendResult, error) {
	granularity := params.Granularity
	if granularity != "daily" && granularity != "monthly" {
		granularity = "monthly"
	}

	items, err := h.repo.GetTrend(ctx, params.UserID, granularity, params.From, params.To, params.Currency)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []domain.TrendItem{}
	}

	cp, _ := h.repo.GetProjectionCheckpoint(ctx, "financialcore")
	var freshness *time.Time
	if cp != nil && !cp.CheckpointAt.IsZero() {
		freshness = &cp.CheckpointAt
	}

	return &TrendResult{
		Items:               items,
		ProjectionFreshness: freshness,
	}, nil
}
