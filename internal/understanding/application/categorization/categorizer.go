package categorization

import (
	"context"
	"strings"
	"sync"
)

// Categorization sources per 11_ai_blueprint.md §7
const (
	SourceUserHistory   = "user_history"
	SourceGlobalMapping = "global_mapping"
	SourceAIInference   = "ai_inference"
	SourceDefault       = "default"

	CategoryUncategorized = "Uncategorized"
)

// Assignment represents a categorized merchant outcome.
type Assignment struct {
	Category   string
	Confidence float64
	Source     string
}

// UserHistoryStore defines retrieval and update of user-specific merchant categories.
type UserHistoryStore interface {
	GetCategory(ctx context.Context, userID, normalizedMerchant string) (*Assignment, error)
	SaveCategory(ctx context.Context, userID, normalizedMerchant, category string) error
}

// GlobalMappingStore defines retrieval and periodic aggregation of platform-wide mappings.
type GlobalMappingStore interface {
	GetCategory(ctx context.Context, normalizedMerchant string) (*Assignment, error)
	SetCategory(ctx context.Context, normalizedMerchant, category string, confidence float64) error
	RefreshAggregations(ctx context.Context) error
}

// Engine implements the 4-tier categorization decision hierarchy (11_ai_blueprint.md §7) [GATE].
type Engine struct {
	userHistory UserHistoryStore
	globalMap   GlobalMappingStore
	aiThreshold float64
}

// Config configures the categorization engine.
type Config struct {
	UserHistory   UserHistoryStore
	GlobalMapping GlobalMappingStore
	AIThreshold   float64
}

// NewEngine creates a new 4-tier Categorization Engine.
func NewEngine(cfg Config) *Engine {
	thresh := cfg.AIThreshold
	if thresh <= 0 {
		thresh = 0.40
	}
	return &Engine{
		userHistory: cfg.UserHistory,
		globalMap:   cfg.GlobalMapping,
		aiThreshold: thresh,
	}
}

// Request holds inputs for assigning a category.
type Request struct {
	UserID             string
	NormalizedMerchant string
	CategoryHint       string
	CategoryConfidence float64
}

// AssignCategory evaluates the 4-tier decision hierarchy in strict order [GATE]:
// 1. user_history -> 2. global_mapping -> 3. ai_inference -> 4. Uncategorized fallback
func (e *Engine) AssignCategory(ctx context.Context, req Request) Assignment {
	cleanMerchant := strings.ToLower(strings.TrimSpace(req.NormalizedMerchant))

	// Tier 1: user_history lookup against confirmed past assignments
	if e.userHistory != nil && cleanMerchant != "" && req.UserID != "" {
		if assign, err := e.userHistory.GetCategory(ctx, req.UserID, cleanMerchant); err == nil && assign != nil {
			return Assignment{
				Category:   assign.Category,
				Confidence: 0.90, // High confidence for explicit user preference
				Source:     SourceUserHistory,
			}
		}
	}

	// Tier 2: global_mapping platform-wide dictionary
	if e.globalMap != nil && cleanMerchant != "" {
		if assign, err := e.globalMap.GetCategory(ctx, cleanMerchant); err == nil && assign != nil {
			return Assignment{
				Category:   assign.Category,
				Confidence: 0.75, // Medium-high confidence for platform consensus
				Source:     SourceGlobalMapping,
			}
		}
	}

	// Tier 3: ai_inference (categoryHint extracted by LLM)
	hint := strings.TrimSpace(req.CategoryHint)
	if hint != "" && req.CategoryConfidence >= e.aiThreshold {
		return Assignment{
			Category:   hint,
			Confidence: req.CategoryConfidence,
			Source:     SourceAIInference,
		}
	}

	// Tier 4: Uncategorized fallback (never block finalization on uncertain category)
	return Assignment{
		Category:   CategoryUncategorized,
		Confidence: 0.0,
		Source:     SourceDefault,
	}
}

// RecordUserCorrection feeds a user's manual category override back into user_history (11_ai_blueprint.md §7 Feedback Loop).
func (e *Engine) RecordUserCorrection(ctx context.Context, userID, normalizedMerchant, category string) error {
	cleanMerchant := strings.ToLower(strings.TrimSpace(normalizedMerchant))
	if e.userHistory != nil && cleanMerchant != "" && userID != "" {
		return e.userHistory.SaveCategory(ctx, userID, cleanMerchant, category)
	}
	return nil
}

// InMemoryStores provides a thread-safe in-memory and fallback implementation of user history and global mappings.
type InMemoryUserHistoryStore struct {
	mu   sync.RWMutex
	data map[string]string // key: userID + ":" + normalizedMerchant -> category
}

func NewInMemoryUserHistoryStore() *InMemoryUserHistoryStore {
	return &InMemoryUserHistoryStore{data: make(map[string]string)}
}

func (s *InMemoryUserHistoryStore) GetCategory(_ context.Context, userID, normalizedMerchant string) (*Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := userID + ":" + strings.ToLower(normalizedMerchant)
	if cat, ok := s.data[key]; ok {
		return &Assignment{Category: cat, Confidence: 0.90, Source: SourceUserHistory}, nil
	}
	return nil, nil
}

func (s *InMemoryUserHistoryStore) SaveCategory(_ context.Context, userID, normalizedMerchant, category string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userID + ":" + strings.ToLower(normalizedMerchant)
	s.data[key] = category
	return nil
}

type InMemoryGlobalMappingStore struct {
	mu   sync.RWMutex
	data map[string]string // key: normalizedMerchant -> category
}

func NewInMemoryGlobalMappingStore() *InMemoryGlobalMappingStore {
	store := &InMemoryGlobalMappingStore{data: make(map[string]string)}
	// Seed standard platform-wide global mappings
	store.data["starbucks"] = "Dining"
	store.data["mcdonalds"] = "Dining"
	store.data["mcdonald's"] = "Dining"
	store.data["kopi kenangan"] = "Food & Beverage"
	store.data["netflix"] = "Subscription"
	store.data["spotify"] = "Subscription"
	store.data["amazon"] = "Shopping"
	store.data["amazon.com"] = "Shopping"
	store.data["tokopedia"] = "Shopping"
	store.data["shopee"] = "Shopping"
	store.data["pln"] = "Utilities"
	store.data["grab"] = "Transportation"
	store.data["uber"] = "Transportation"
	store.data["gojek"] = "Transportation"
	return store
}

func (s *InMemoryGlobalMappingStore) GetCategory(_ context.Context, normalizedMerchant string) (*Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cat, ok := s.data[strings.ToLower(normalizedMerchant)]; ok {
		return &Assignment{Category: cat, Confidence: 0.75, Source: SourceGlobalMapping}, nil
	}
	return nil, nil
}

func (s *InMemoryGlobalMappingStore) SetCategory(_ context.Context, normalizedMerchant, category string, _ float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[strings.ToLower(normalizedMerchant)] = category
	return nil
}

func (s *InMemoryGlobalMappingStore) RefreshAggregations(_ context.Context) error {
	// Periodic privacy-preserving aggregation job (11_ai_blueprint.md §7)
	return nil
}
