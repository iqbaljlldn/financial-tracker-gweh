package domain

import "errors"

var (
	// ErrUserNotFound indicates that the requested user does not exist.
	ErrUserNotFound = errors.New("identity: user not found")

	// ErrEmailAlreadyLinked enforces H1 (07_sequence_diagrams.md Diagram 1, 08_database_design.md §8.1):
	// an email address cannot be linked to more than one platform account.
	ErrEmailAlreadyLinked = errors.New("identity: email address is already linked to another account")

	// ErrInvalidEmail indicates that the email address format is invalid.
	ErrInvalidEmail = errors.New("identity: invalid email address format")

	// ErrOAuthTokenNotFound indicates no OAuth token record exists for the given Gmail account.
	ErrOAuthTokenNotFound = errors.New("identity: oauth token not found")

	// ErrTokenExpired indicates that the access token has expired and must be refreshed.
	ErrTokenExpired = errors.New("identity: oauth access token has expired")

	// ErrRevocationDetected indicates Google returned invalid_grant, meaning user revoked consent (12_gmail_connector.md §4.3).
	ErrRevocationDetected = errors.New("identity: oauth grant revoked by user")

	// ErrUnauthorized indicates an ownership mismatch or missing credentials.
	ErrUnauthorized = errors.New("identity: unauthorized")
)
