package query

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

type CashFlowResult struct {
	Buckets             []domain.CashFlowBucket `json:"buckets"`
	ProjectionFreshness *time.Time              `json:"projectionFreshness"`
}

type GetCashFlowHandler struct {
	repo domain.Repository
}

func NewGetCashFlowHandler(repo domain.Repository) *GetCashFlowHandler {
	return &GetCashFlowHandler{repo: repo}
}

type GetCashFlowParams struct {
	UserID      string
	Granularity string
	From        *time.Time
	To          *time.Time
	Currency    *string
}

func (h *GetCashFlowHandler) Handle(ctx context.Context, params GetCashFlowParams) (*CashFlowResult, error) {
	granularity := params.Granularity
	if granularity != "daily" && granularity != "monthly" {
		granularity = "monthly"
	}

	buckets, err := h.repo.GetCashFlow(ctx, params.UserID, granularity, params.From, params.To, params.Currency)
	if err != nil {
		return nil, err
	}
	if buckets == nil {
		buckets = []domain.CashFlowBucket{}
	}

	cp, _ := h.repo.GetProjectionCheckpoint(ctx, "financialcore")
	var freshness *time.Time
	if cp != nil && !cp.CheckpointAt.IsZero() {
		freshness = &cp.CheckpointAt
	}

	return &CashFlowResult{
		Buckets:             buckets,
		ProjectionFreshness: freshness,
	}, nil
}
