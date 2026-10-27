package domain

import (
	"encoding/json"
	"time"
)

// AuditLogEntry represents an immutable record in the append-only audit trail
// (08_database_design.md §13.1, 16_security.md §8).
// Audit log records are explicitly exempt from GDPR erasure cascades.
type AuditLogEntry struct {
	id         string
	userID     *string
	action     string
	actor      string
	metadata   map[string]any
	occurredAt time.Time
}

// NewAuditLogEntry constructs a domain AuditLogEntry.
func NewAuditLogEntry(
	id string,
	userID *string,
	action string,
	actor string,
	metadata map[string]any,
	occurredAt time.Time,
) *AuditLogEntry {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return &AuditLogEntry{
		id:         id,
		userID:     userID,
		action:     action,
		actor:      actor,
		metadata:   metadata,
		occurredAt: occurredAt,
	}
}

func (e *AuditLogEntry) ID() string               { return e.id }
func (e *AuditLogEntry) UserID() *string          { return e.userID }
func (e *AuditLogEntry) Action() string           { return e.action }
func (e *AuditLogEntry) Actor() string            { return e.actor }
func (e *AuditLogEntry) Metadata() map[string]any { return e.metadata }
func (e *AuditLogEntry) OccurredAt() time.Time    { return e.occurredAt }

func (e *AuditLogEntry) MetadataJSON() ([]byte, error) {
	return json.Marshal(e.metadata)
}

// AuditLogFilter models query criteria for GET /admin/audit-log per 09_api_specification.md §6.6.
type AuditLogFilter struct {
	UserID   *string
	Action   *string
	FromTime *time.Time
	ToTime   *time.Time
	Limit    int
	Offset   int
}
