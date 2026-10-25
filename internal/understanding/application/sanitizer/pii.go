package sanitizer

import (
	"regexp"
	"strings"
)

var (
	// Regex matching credit card and bank account numbers (10 to 19 digits, with optional spaces or dashes)
	cardOrAccountRegex = regexp.MustCompile(`\b(?:\d[ -]?){9,18}\d\b`)

	// Regex for IBANs: 2 letter country code, 2 check digits, followed by alphanumeric characters
	ibanRegex = regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{4,30}\b`)

	// Regex for boilerplate footer sections (legal disclaimers, unsubscribe, email footers)
	footerPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:\n\s*[-*_]{3,}\s*)?\n\s*(?:to unsubscribe|unsubscribe here|manage your preferences|opt out|click here to unsubscribe)[\s\S]*`),
		regexp.MustCompile(`(?i)(?:\n\s*[-*_]{3,}\s*)?\n\s*(?:this (?:email|message) (?:and any attachments )?is (?:confidential|intended solely|privileged))[\s\S]*`),
		regexp.MustCompile(`(?i)(?:\n\s*[-*_]{3,}\s*)?\n\s*(?:if you (?:have received|are not the intended recipient))[\s\S]*`),
		regexp.MustCompile(`(?i)(?:\n\s*[-*_]{3,}\s*)?\n\s*(?:please do not reply to this email|ini adalah email otomatis, mohon tidak membalas)[\s\S]*`),
	}

	// Trailing divider lines
	trailingDividerRegex = regexp.MustCompile(`(?:\n\s*[-*_]{3,}\s*)+$`)

	// Multiple whitespace / newline collapse
	multiNewlineRegex = regexp.MustCompile(`\n{3,}`)
	multiSpaceRegex   = regexp.MustCompile(`[ \t]{2,}`)
)

const (
	// DefaultMaxCharacters caps the email content to avoid token exhaustion
	DefaultMaxCharacters = 4000
)

// PIIConfig holds sanitization options.
type PIIConfig struct {
	MaxCharacters int
}

// DefaultPIIConfig returns default sanitization options.
func DefaultPIIConfig() PIIConfig {
	return PIIConfig{
		MaxCharacters: DefaultMaxCharacters,
	}
}

// SanitizeEmail removes PII (masks account/card numbers) and strips email boilerplate.
func SanitizeEmail(content string, cfg ...PIIConfig) string {
	maxChars := DefaultMaxCharacters
	if len(cfg) > 0 && cfg[0].MaxCharacters > 0 {
		maxChars = cfg[0].MaxCharacters
	}

	sanitized := content

	// 1. Strip signature & footer boilerplate
	for _, pattern := range footerPatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "")
	}

	// 2. Mask card / bank account numbers (preserving only the last 4 digits)
	sanitized = cardOrAccountRegex.ReplaceAllStringFunc(sanitized, func(match string) string {
		digitsOnly := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, match)

		if len(digitsOnly) >= 10 && len(digitsOnly) <= 19 {
			last4 := digitsOnly[len(digitsOnly)-4:]
			return "****" + last4
		}
		return match
	})

	// 3. Mask IBANs
	sanitized = ibanRegex.ReplaceAllStringFunc(sanitized, func(match string) string {
		if len(match) > 6 {
			return match[:4] + "****" + match[len(match)-4:]
		}
		return match
	})

	// 4. Clean up excessive whitespace and trailing divider lines
	sanitized = trailingDividerRegex.ReplaceAllString(sanitized, "")
	sanitized = multiNewlineRegex.ReplaceAllString(sanitized, "\n\n")
	sanitized = multiSpaceRegex.ReplaceAllString(sanitized, " ")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = trailingDividerRegex.ReplaceAllString(sanitized, "")
	sanitized = strings.TrimSpace(sanitized)

	// 5. Enforce token budget truncation cap (11_ai_blueprint.md §11)
	if len(sanitized) > maxChars {
		sanitized = sanitized[:maxChars] + "\n[Content truncated for token budget]"
	}

	return sanitized
}
