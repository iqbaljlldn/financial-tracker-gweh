package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

// DBTX represents the database transaction or pool interface needed by sqlc.
type DBTX = gen.DBTX

// UserRepository implements domain.UserRepository using sqlc generated queries.
type UserRepository struct {
	queries *gen.Queries
	db      DBTX
}

// NewUserRepository creates a new UserRepository instance.
func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{
		queries: gen.New(db),
		db:      db,
	}
}

var _ domain.UserRepository = (*UserRepository)(nil)

// Save persists a new User aggregate.
func (r *UserRepository) Save(ctx context.Context, u *domain.User) error {
	idUUID, err := uuid.Parse(u.ID())
	if err != nil {
		return fmt.Errorf("identity postgres: invalid user ID: %w", err)
	}

	var dailyTime pgtype.Time
	if u.NotificationDailyTime() != nil {
		t, err := time.Parse("15:04:05", *u.NotificationDailyTime())
		if err == nil {
			dailyTime = pgtype.Time{
				Microseconds: int64(t.Hour()*3600+t.Minute()*60+t.Second()) * 1_000_000,
				Valid:        true,
			}
		}
	}

	params := gen.CreateUserParams{
		ID:    pgtype.UUID{Bytes: idUUID, Valid: true},
		Email: u.Email(),
		DisplayName: pgtype.Text{
			String: u.DisplayName(),
			Valid:  u.DisplayName() != "",
		},
		Timezone:              u.Timezone(),
		NotificationDailyTime: dailyTime,
		CreatedAt:             pgtype.Timestamptz{Time: u.CreatedAt(), Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: u.UpdatedAt(), Valid: true},
	}

	_, err = r.queries.CreateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("identity postgres: save user failed: %w", err)
	}
	return nil
}

// FindByID retrieves a User by unique ID.
// Mandatory userID parameter establishes row-level ownership convention [GATE] (16_security.md §6.2, 20_coding_convention.md §9.2).
func (r *UserRepository) FindByID(ctx context.Context, userID string) (*domain.User, error) {
	idUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}

	row, err := r.queries.GetUserByID(ctx, pgtype.UUID{Bytes: idUUID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("identity postgres: get user by id failed: %w", err)
	}

	return toDomainUser(row), nil
}

// FindByEmail retrieves a User by their email address.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("identity postgres: get user by email failed: %w", err)
	}

	return toDomainUser(row), nil
}

// Update updates an existing User aggregate.
// Mandatory userID parameter enforces row-level ownership [GATE].
func (r *UserRepository) Update(ctx context.Context, userID string, u *domain.User) error {
	idUUID, err := uuid.Parse(userID)
	if err != nil {
		return domain.ErrUserNotFound
	}

	var dailyTime pgtype.Time
	if u.NotificationDailyTime() != nil {
		t, err := time.Parse("15:04:05", *u.NotificationDailyTime())
		if err == nil {
			dailyTime = pgtype.Time{
				Microseconds: int64(t.Hour()*3600+t.Minute()*60+t.Second()) * 1_000_000,
				Valid:        true,
			}
		}
	}

	params := gen.UpdateUserSettingsParams{
		ID: pgtype.UUID{Bytes: idUUID, Valid: true},
		DisplayName: pgtype.Text{
			String: u.DisplayName(),
			Valid:  u.DisplayName() != "",
		},
		Timezone:              u.Timezone(),
		NotificationDailyTime: dailyTime,
		UpdatedAt:             pgtype.Timestamptz{Time: u.UpdatedAt(), Valid: true},
	}

	_, err = r.queries.UpdateUserSettings(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("identity postgres: update user settings failed: %w", err)
	}
	return nil
}

// --- OAuthTokenRepository ---

// OAuthTokenRepository implements domain.OAuthTokenRepository using sqlc.
type OAuthTokenRepository struct {
	queries *gen.Queries
	db      DBTX
}

// NewOAuthTokenRepository creates a new OAuthTokenRepository instance.
func NewOAuthTokenRepository(db DBTX) *OAuthTokenRepository {
	return &OAuthTokenRepository{
		queries: gen.New(db),
		db:      db,
	}
}

var _ domain.OAuthTokenRepository = (*OAuthTokenRepository)(nil)

// Save persists a new OAuthToken entity.
func (r *OAuthTokenRepository) Save(ctx context.Context, t *domain.OAuthToken) error {
	idUUID, err := uuid.Parse(t.ID())
	if err != nil {
		return fmt.Errorf("identity postgres: invalid token ID: %w", err)
	}
	gmailAccountUUID, err := uuid.Parse(t.GmailAccountID())
	if err != nil {
		return fmt.Errorf("identity postgres: invalid gmail account ID: %w", err)
	}

	params := gen.CreateOAuthTokenParams{
		ID:                    pgtype.UUID{Bytes: idUUID, Valid: true},
		GmailAccountID:        pgtype.UUID{Bytes: gmailAccountUUID, Valid: true},
		EncryptedAccessToken:  t.EncryptedAccessToken(),
		EncryptedRefreshToken: t.EncryptedRefreshToken(),
		AccessTokenExpiresAt:  pgtype.Timestamptz{Time: t.AccessTokenExpiresAt(), Valid: true},
		Scope:                 t.Scope(),
		CreatedAt:             pgtype.Timestamptz{Time: t.CreatedAt(), Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: t.UpdatedAt(), Valid: true},
	}

	_, err = r.queries.CreateOAuthToken(ctx, params)
	if err != nil {
		return fmt.Errorf("identity postgres: save oauth token failed: %w", err)
	}
	return nil
}

// FindByGmailAccountID retrieves the OAuth token for a given Gmail account.
func (r *OAuthTokenRepository) FindByGmailAccountID(ctx context.Context, gmailAccountID string) (*domain.OAuthToken, error) {
	gmailUUID, err := uuid.Parse(gmailAccountID)
	if err != nil {
		return nil, domain.ErrOAuthTokenNotFound
	}

	row, err := r.queries.GetOAuthTokenByGmailAccountID(ctx, pgtype.UUID{Bytes: gmailUUID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrOAuthTokenNotFound
		}
		return nil, fmt.Errorf("identity postgres: get oauth token failed: %w", err)
	}

	return domain.ReconstituteOAuthToken(
		uuid.UUID(row.ID.Bytes).String(),
		uuid.UUID(row.GmailAccountID.Bytes).String(),
		row.EncryptedAccessToken,
		row.EncryptedRefreshToken,
		row.AccessTokenExpiresAt.Time,
		row.Scope,
		row.CreatedAt.Time,
		row.UpdatedAt.Time,
	), nil
}

// Update updates an existing OAuthToken entity with new token bytes and expiration.
func (r *OAuthTokenRepository) Update(ctx context.Context, t *domain.OAuthToken) error {
	idUUID, err := uuid.Parse(t.ID())
	if err != nil {
		return domain.ErrOAuthTokenNotFound
	}

	params := gen.UpdateOAuthTokenParams{
		ID:                    pgtype.UUID{Bytes: idUUID, Valid: true},
		EncryptedAccessToken:  t.EncryptedAccessToken(),
		EncryptedRefreshToken: t.EncryptedRefreshToken(),
		AccessTokenExpiresAt:  pgtype.Timestamptz{Time: t.AccessTokenExpiresAt(), Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: t.UpdatedAt(), Valid: true},
	}

	_, err = r.queries.UpdateOAuthToken(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrOAuthTokenNotFound
		}
		return fmt.Errorf("identity postgres: update oauth token failed: %w", err)
	}
	return nil
}

// --- RefreshTokenRepository ---

// RefreshTokenRepository implements domain.RefreshTokenRepository using sqlc.
type RefreshTokenRepository struct {
	queries *gen.Queries
	db      DBTX
}

// NewRefreshTokenRepository creates a new RefreshTokenRepository instance.
func NewRefreshTokenRepository(db DBTX) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		queries: gen.New(db),
		db:      db,
	}
}

var _ domain.RefreshTokenRepository = (*RefreshTokenRepository)(nil)

// Save stores a new hashed platform refresh token for a user.
// Mandatory userID parameter establishes row-level ownership [GATE].
func (r *RefreshTokenRepository) Save(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("identity postgres: invalid user ID: %w", err)
	}

	params := gen.CreateRefreshTokenParams{
		ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
		UserID:    pgtype.UUID{Bytes: userUUID, Valid: true},
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	_, err = r.queries.CreateRefreshToken(ctx, params)
	if err != nil {
		return fmt.Errorf("identity postgres: save refresh token failed: %w", err)
	}
	return nil
}

// FindValidByHash finds a non-revoked, non-expired refresh token by its SHA-256 hash.
func (r *RefreshTokenRepository) FindValidByHash(ctx context.Context, tokenHash string) (string, error) {
	row, err := r.queries.GetValidRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrUnauthorized
		}
		return "", fmt.Errorf("identity postgres: get refresh token failed: %w", err)
	}

	return uuid.UUID(row.UserID.Bytes).String(), nil
}

// Revoke marks a user's refresh token as revoked.
// Mandatory userID parameter enforces row-level ownership [GATE].
func (r *RefreshTokenRepository) Revoke(ctx context.Context, userID string, tokenHash string) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return domain.ErrUnauthorized
	}

	params := gen.RevokeRefreshTokenParams{
		UserID:    pgtype.UUID{Bytes: userUUID, Valid: true},
		TokenHash: tokenHash,
	}

	_, err = r.queries.RevokeRefreshToken(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return fmt.Errorf("identity postgres: revoke refresh token failed: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes all platform refresh tokens for a user.
// Mandatory userID parameter enforces row-level ownership [GATE].
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return domain.ErrUnauthorized
	}

	err = r.queries.RevokeAllUserRefreshTokens(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		return fmt.Errorf("identity postgres: revoke all refresh tokens failed: %w", err)
	}
	return nil
}

// toDomainUser translates a sqlc IdentityUser row to a domain User aggregate.
func toDomainUser(row gen.IdentityUser) *domain.User {
	var dailyTime *string
	if row.NotificationDailyTime.Valid {
		totalSeconds := row.NotificationDailyTime.Microseconds / 1_000_000
		hour := totalSeconds / 3600
		min := (totalSeconds % 3600) / 60
		sec := totalSeconds % 60
		formatted := fmt.Sprintf("%02d:%02d:%02d", hour, min, sec)
		dailyTime = &formatted
	}

	displayName := ""
	if row.DisplayName.Valid {
		displayName = row.DisplayName.String
	}

	return domain.ReconstituteUser(
		uuid.UUID(row.ID.Bytes).String(),
		row.Email,
		displayName,
		row.Timezone,
		dailyTime,
		row.CreatedAt.Time,
		row.UpdatedAt.Time,
	)
}
