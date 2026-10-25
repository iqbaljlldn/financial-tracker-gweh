package domain

import (
	"strings"
	"time"
)

// User represents the User aggregate root in the Identity & Access context per 04_domain_model.md §8.1.
type User struct {
	id                    string
	email                 string
	displayName           string
	timezone              string
	notificationDailyTime *string
	createdAt             time.Time
	updatedAt             time.Time
}

// NewUser constructs and validates a new User aggregate.
func NewUser(id, email, displayName, timezone string) (*User, error) {
	trimmedEmail := strings.TrimSpace(strings.ToLower(email))
	if trimmedEmail == "" || !strings.Contains(trimmedEmail, "@") {
		return nil, ErrInvalidEmail
	}

	if timezone == "" {
		timezone = "UTC"
	}

	now := time.Now().UTC()
	return &User{
		id:          id,
		email:       trimmedEmail,
		displayName: strings.TrimSpace(displayName),
		timezone:    timezone,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// ReconstituteUser rebuilds a User aggregate from persistence storage without re-triggering creation invariants.
func ReconstituteUser(
	id, email, displayName, timezone string,
	notificationDailyTime *string,
	createdAt, updatedAt time.Time,
) *User {
	return &User{
		id:                    id,
		email:                 email,
		displayName:           displayName,
		timezone:              timezone,
		notificationDailyTime: notificationDailyTime,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
	}
}

// ID returns the user's globally unique identifier.
func (u *User) ID() string {
	return u.id
}

// Email returns the user's primary email address.
func (u *User) Email() string {
	return u.email
}

// DisplayName returns the user's display name.
func (u *User) DisplayName() string {
	return u.displayName
}

// Timezone returns the user's configured timezone.
func (u *User) Timezone() string {
	return u.timezone
}

// NotificationDailyTime returns the daily summary dispatch time if configured.
func (u *User) NotificationDailyTime() *string {
	return u.notificationDailyTime
}

// CreatedAt returns the user creation timestamp.
func (u *User) CreatedAt() time.Time {
	return u.createdAt
}

// UpdatedAt returns the user last updated timestamp.
func (u *User) UpdatedAt() time.Time {
	return u.updatedAt
}

// UpdateSettings updates user configurable settings per 04_domain_model.md §8.1.
func (u *User) UpdateSettings(displayName, timezone string, dailyTime *string) {
	if displayName != "" {
		u.displayName = strings.TrimSpace(displayName)
	}
	if timezone != "" {
		u.timezone = timezone
	}
	if dailyTime != nil {
		u.notificationDailyTime = dailyTime
	}
	u.updatedAt = time.Now().UTC()
}
