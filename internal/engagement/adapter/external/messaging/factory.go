package messaging

import (
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/fonnte"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/meta"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/mock"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// ProviderConfig aggregates configuration for messaging providers.
type ProviderConfig struct {
	Provider string // "fonnte", "meta", "mock"
	Fonnte   fonnte.Config
	Meta     meta.Config
}

// NewMessagingClient returns the appropriate MessagingClient based on configuration.
func NewMessagingClient(cfg ProviderConfig) domain.MessagingClient {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "meta":
		return meta.New(cfg.Meta)
	case "mock":
		return mock.New()
	case "fonnte":
		fallthrough
	default:
		return fonnte.New(cfg.Fonnte)
	}
}
