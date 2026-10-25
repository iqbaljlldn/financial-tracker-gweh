package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"github.com/redis/go-redis/v9"
)

const activeTemplatesCacheKey = "understanding:active_templates"
const cacheTTL = 10 * time.Minute

// CachedTemplateDTO represents the serialized template for Redis caching (06_architecture.md §13).
type CachedTemplateDTO struct {
	ID            string                 `json:"id"`
	SenderPattern string                 `json:"senderPattern"`
	Version       int                    `json:"version"`
	Rules         domain.ExtractionRules `json:"rules"`
	Active        bool                   `json:"active"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// TemplateMatcher resolves the active SenderTemplate for a given sender email address.
type TemplateMatcher struct {
	repo        domain.SenderTemplateRepository
	redisClient *redis.Client
	logger      *slog.Logger
}

// NewTemplateMatcher creates a new TemplateMatcher with Redis cache support.
func NewTemplateMatcher(
	repo domain.SenderTemplateRepository,
	redisClient *redis.Client,
	logger *slog.Logger,
) *TemplateMatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &TemplateMatcher{
		repo:        repo,
		redisClient: redisClient,
		logger:      logger,
	}
}

// MatchSender finds the active template that matches the sender email.
// Checks Redis cache first; on miss, fetches from DB and populates cache.
func (m *TemplateMatcher) MatchSender(ctx context.Context, senderEmail string) (*domain.SenderTemplate, error) {
	templates, err := m.getActiveTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active templates: %w", err)
	}

	for _, tmpl := range templates {
		if tmpl.Matches(senderEmail) {
			return tmpl, nil
		}
	}

	return nil, domain.ErrTemplateNotFound
}

// InvalidateCache invalidates the active template list in Redis.
func (m *TemplateMatcher) InvalidateCache(ctx context.Context) {
	if m.redisClient == nil {
		return
	}
	if err := m.redisClient.Del(ctx, activeTemplatesCacheKey).Err(); err != nil {
		m.logger.Warn("failed to invalidate active templates cache", slog.String("error", err.Error()))
	}
}

func (m *TemplateMatcher) getActiveTemplates(ctx context.Context) ([]*domain.SenderTemplate, error) {
	// Try Redis cache if available
	if m.redisClient != nil {
		val, err := m.redisClient.Get(ctx, activeTemplatesCacheKey).Result()
		if err == nil && val != "" {
			var dtos []CachedTemplateDTO
			if unmarshalErr := json.Unmarshal([]byte(val), &dtos); unmarshalErr == nil {
				var result []*domain.SenderTemplate
				for _, dto := range dtos {
					t, err := domain.NewSenderTemplate(
						dto.ID,
						dto.SenderPattern,
						dto.Version,
						dto.Rules,
						dto.Active,
						dto.CreatedAt,
					)
					if err == nil {
						result = append(result, t)
					}
				}
				if len(result) > 0 {
					return result, nil
				}
			}
		}
	}

	// Fetch from database
	templates, err := m.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	// Populate Redis cache
	if m.redisClient != nil && len(templates) > 0 {
		var dtos []CachedTemplateDTO
		for _, t := range templates {
			dtos = append(dtos, CachedTemplateDTO{
				ID:            t.ID(),
				SenderPattern: t.SenderPattern(),
				Version:       t.Version(),
				Rules:         t.Rules(),
				Active:        t.Active(),
				CreatedAt:     t.CreatedAt(),
			})
		}
		if bytes, err := json.Marshal(dtos); err == nil {
			_ = m.redisClient.Set(ctx, activeTemplatesCacheKey, string(bytes), cacheTTL).Err()
		}
	}

	return templates, nil
}
