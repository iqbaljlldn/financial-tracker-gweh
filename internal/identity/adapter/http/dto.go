package http

import (
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

// InitiateAuthResponse matches 09_api_specification.md §5.1.
type InitiateAuthResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
}

// UserDTO matches 09_api_specification.md §5.1 and §6.1.
type UserDTO struct {
	ID                    string  `json:"id"`
	Email                 string  `json:"email"`
	DisplayName           string  `json:"displayName"`
	Timezone              string  `json:"timezone,omitempty"`
	NotificationDailyTime *string `json:"notificationDailyTime,omitempty"`
}

// CallbackAuthResponse matches 09_api_specification.md §5.1.
type CallbackAuthResponse struct {
	SessionToken string  `json:"sessionToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresAt    string  `json:"expiresAt"`
	User         UserDTO `json:"user"`
}

// RefreshSessionRequest represents the body for POST /auth/refresh.
type RefreshSessionRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// RefreshSessionResponse represents the response for POST /auth/refresh.
type RefreshSessionResponse struct {
	SessionToken string `json:"sessionToken"`
	ExpiresAt    string `json:"expiresAt"`
}

// UpdateSettingsRequest matches 09_api_specification.md §6.1 PATCH /me/settings.
type UpdateSettingsRequest struct {
	DisplayName           *string `json:"displayName,omitempty"`
	Timezone              *string `json:"timezone,omitempty"`
	NotificationDailyTime *string `json:"notificationDailyTime,omitempty"`
}

// ErrorEnvelope matches 09_api_specification.md §7.2 standard error shape.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes the error payload.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// ToUserDTO translates a domain User aggregate to its wire representation.
func ToUserDTO(u *domain.User) UserDTO {
	return UserDTO{
		ID:                    u.ID(),
		Email:                 u.Email(),
		DisplayName:           u.DisplayName(),
		Timezone:              u.Timezone(),
		NotificationDailyTime: u.NotificationDailyTime(),
	}
}

// FormatRFC3339 formats a time.Time in RFC3339 string.
func FormatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
