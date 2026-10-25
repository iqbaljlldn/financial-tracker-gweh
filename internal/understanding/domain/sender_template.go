package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrEmptySenderPattern = errors.New("sender template: senderPattern is required")
	ErrInvalidVersion     = errors.New("sender template: version must be >= 1")
	ErrMissingRuleField   = errors.New("sender template: missing required rule field definition")
)

// SenderTemplate aggregate root per 04_domain_model.md §7.1 & 13_email_parser.md §4-5.
type SenderTemplate struct {
	id            string
	senderPattern string
	version       int
	rules         ExtractionRules
	active        bool
	createdAt     time.Time
}

// NewSenderTemplate creates and validates a SenderTemplate aggregate root.
func NewSenderTemplate(
	id string,
	senderPattern string,
	version int,
	rules ExtractionRules,
	active bool,
	createdAt time.Time,
) (*SenderTemplate, error) {
	cleanPattern := strings.ToLower(strings.TrimSpace(senderPattern))
	if cleanPattern == "" {
		return nil, ErrEmptySenderPattern
	}
	if version < 1 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidVersion, version)
	}

	// Verify minimum field rule presence
	if rules.Fields == nil {
		rules.Fields = make(map[string]FieldRule)
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return &SenderTemplate{
		id:            id,
		senderPattern: cleanPattern,
		version:       version,
		rules:         rules,
		active:        active,
		createdAt:     createdAt,
	}, nil
}

// ID returns the template UUID.
func (t *SenderTemplate) ID() string {
	return t.id
}

// SenderPattern returns the pattern expression.
func (t *SenderTemplate) SenderPattern() string {
	return t.senderPattern
}

// Version returns monotonic template version.
func (t *SenderTemplate) Version() int {
	return t.version
}

// Rules returns declarative extraction rules.
func (t *SenderTemplate) Rules() ExtractionRules {
	return t.rules
}

// Active returns true if this template is currently active for extraction.
func (t *SenderTemplate) Active() bool {
	return t.active
}

// SetActive updates active status.
func (t *SenderTemplate) SetActive(active bool) {
	t.active = active
}

// CreatedAt returns creation timestamp.
func (t *SenderTemplate) CreatedAt() time.Time {
	return t.createdAt
}

// Matches checks whether an email address matches this template's pattern.
// Supports exact match, domain match, and wildcard subdomain/user match.
// e.g. "no-reply@grab.com", "*@bca.co.id", "*.notifications@examplebank.com".
func (t *SenderTemplate) Matches(senderEmail string) bool {
	cleanSender := strings.ToLower(strings.TrimSpace(senderEmail))
	pattern := strings.ToLower(strings.TrimSpace(t.senderPattern))

	if cleanSender == "" || pattern == "" {
		return false
	}

	// Extract email address if format is "Display Name <email@domain.com>"
	if start := strings.Index(cleanSender, "<"); start != -1 {
		if end := strings.LastIndex(cleanSender, ">"); end > start {
			cleanSender = cleanSender[start+1 : end]
		}
	}

	// Case 1: Exact match
	if cleanSender == pattern {
		return true
	}

	// Case 2: Standard wildcard matching using filepath.Match
	if matched, err := filepath.Match(pattern, cleanSender); err == nil && matched {
		return true
	}

	// Case 3: Domain match suffix (e.g. pattern "@bca.co.id" or "*@bca.co.id")
	if strings.HasPrefix(pattern, "@") || strings.HasPrefix(pattern, "*@") {
		domainPart := strings.TrimPrefix(pattern, "*") // e.g. "@bca.co.id"
		if strings.HasSuffix(cleanSender, domainPart) {
			return true
		}
		// Also support subdomains: e.g. receipts@gotagihan.gojek.com matching *@gojek.com
		bareDomain := strings.TrimPrefix(domainPart, "@")
		atIdx := strings.LastIndex(cleanSender, "@")
		if atIdx != -1 {
			senderDomain := cleanSender[atIdx+1:]
			if senderDomain == bareDomain || strings.HasSuffix(senderDomain, "."+bareDomain) {
				return true
			}
		}
	}

	return false
}
