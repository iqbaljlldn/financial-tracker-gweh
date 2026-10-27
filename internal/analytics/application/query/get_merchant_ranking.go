package query

import (
	"context"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
)

type MerchantRankingResult struct {
	Items               []domain.MerchantRankingItem `json:"items"`
	ProjectionFreshness *time.Time                   `json:"projectionFreshness"`
}

type GetMerchantRankingHandler struct {
	repo domain.Repository
}

func NewGetMerchantRankingHandler(repo domain.Repository) *GetMerchantRankingHandler {
	return &GetMerchantRankingHandler{repo: repo}
}

type GetMerchantRankingParams struct {
	UserID   string
	From     *time.Time
	To       *time.Time
	Currency *string
	Limit    int32
}

func (h *GetMerchantRankingHandler) Handle(ctx context.Context, params GetMerchantRankingParams) (*MerchantRankingResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}

	items, err := h.repo.GetMerchantRanking(ctx, params.UserID, params.From, params.To, params.Currency, limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []domain.MerchantRankingItem{}
	}

	cp, _ := h.repo.GetProjectionCheckpoint(ctx, "financialcore")
	var freshness *time.Time
	if cp != nil && !cp.CheckpointAt.IsZero() {
		freshness = &cp.CheckpointAt
	}

	return &MerchantRankingResult{
		Items:               items,
		ProjectionFreshness: freshness,
	}, nil
}
