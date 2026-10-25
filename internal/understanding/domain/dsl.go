package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrUnknownSelectorType            = errors.New("dsl: unknown selector type")
	ErrUnknownPostProcessingPrimitive = errors.New("dsl: unknown postProcessing primitive (closed enumerated set violated)")
	ErrPostProcessingFailed           = errors.New("dsl: postProcessing failed")
	ErrMalformedDate                  = errors.New("dsl: malformed date string")
	ErrMalformedDecimal               = errors.New("dsl: malformed decimal string")
)

// SelectorType defines supported extraction selectors per 13_email_parser.md §4.2
type SelectorType string

const (
	SelectorCSS    SelectorType = "css"
	SelectorRegex  SelectorType = "regex"
	SelectorHeader SelectorType = "header"
	SelectorStatic SelectorType = "static"
)

// Valid postProcessing primitive names (closed enumerated set per 13_email_parser.md §4.2)
const (
	PostProcessingTrimWhitespace      = "trimWhitespace"
	PostProcessingParseDecimal        = "parseDecimal"
	PostProcessingParseDate           = "parseDate"
	PostProcessingStripCurrencySymbol = "stripCurrencySymbol"
	PostProcessingTitleCase           = "titleCase"
)

// FieldRule represents extraction configuration for a single field.
// Stored as JSONB in PostgreSQL (never executable code per 13_email_parser.md §4.1).
type FieldRule struct {
	SelectorType       SelectorType `json:"selectorType"`
	SelectorExpression string       `json:"selectorExpression,omitempty"`
	ExtractionGroup    int          `json:"extractionGroup,omitempty"`
	Attribute          string       `json:"attribute,omitempty"`
	StaticValue        string       `json:"staticValue,omitempty"`
	PostProcessing     []string     `json:"postProcessing,omitempty"`
	Required           bool         `json:"required"`
}

// ExtractionRules represents the full declarative rule set for a SenderTemplate.
type ExtractionRules struct {
	SenderPattern string               `json:"senderPattern"`
	Version       int                  `json:"version"`
	Fields        map[string]FieldRule `json:"fields"`
}

var whitespaceRegex = regexp.MustCompile(`\s+`)
var currencySymbolsRegex = regexp.MustCompile(`(?i)(rp\.?|idr|\$|usd|€|eur|sgd|s\$|¥)`)

// ApplyPostProcessing executes a closed sequence of transformations against a raw extracted string.
func ApplyPostProcessing(raw string, steps []string) (string, error) {
	current := raw
	for _, step := range steps {
		stepName := step
		var stepParam string
		if idx := strings.Index(step, ":"); idx != -1 {
			stepName = step[:idx]
			stepParam = step[idx+1:]
		}

		switch stepName {
		case PostProcessingTrimWhitespace:
			current = TrimWhitespace(current)
		case PostProcessingStripCurrencySymbol:
			current = StripCurrencySymbol(current)
		case PostProcessingParseDecimal:
			parsed, err := ParseDecimal(current)
			if err != nil {
				return "", fmt.Errorf("%w: %v for %q", ErrMalformedDecimal, err, current)
			}
			current = parsed
		case PostProcessingParseDate:
			parsed, err := ParseDate(current, stepParam)
			if err != nil {
				return "", fmt.Errorf("%w: %v for %q", ErrMalformedDate, err, current)
			}
			current = parsed
		case PostProcessingTitleCase:
			current = TitleCase(current)
		default:
			return "", fmt.Errorf("%w: %q", ErrUnknownPostProcessingPrimitive, stepName)
		}
	}
	return current, nil
}

// TrimWhitespace removes leading/trailing spaces and collapses internal whitespace.
func TrimWhitespace(val string) string {
	trimmed := strings.TrimSpace(val)
	return whitespaceRegex.ReplaceAllString(trimmed, " ")
}

// StripCurrencySymbol removes currency identifiers (Rp, IDR, $, USD, EUR, SGD, etc.) and surrounding punctuation.
func StripCurrencySymbol(val string) string {
	cleaned := currencySymbolsRegex.ReplaceAllString(val, "")
	return strings.TrimSpace(cleaned)
}

// ParseDecimal normalizes locale-specific decimal formats into standard float string representation (e.g. "1250000.50").
// Correctly handles Indonesian/European ("1.250.000,50") and US/UK ("1,250,000.50") formats.
func ParseDecimal(val string) (string, error) {
	cleaned := strings.TrimSpace(val)
	// Remove any remaining spaces
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	if cleaned == "" {
		return "", errors.New("empty string")
	}

	hasDot := strings.Contains(cleaned, ".")
	hasComma := strings.Contains(cleaned, ",")

	var standard string
	if hasDot && hasComma {
		lastDot := strings.LastIndex(cleaned, ".")
		lastComma := strings.LastIndex(cleaned, ",")
		if lastComma > lastDot {
			// Indonesian / European: 1.250.000,50
			s := strings.ReplaceAll(cleaned, ".", "")
			standard = strings.ReplaceAll(s, ",", ".")
		} else {
			// US / UK: 1,250,000.50
			standard = strings.ReplaceAll(cleaned, ",", "")
		}
	} else if hasComma {
		// Only comma present
		lastComma := strings.LastIndex(cleaned, ",")
		afterComma := cleaned[lastComma+1:]
		if len(afterComma) == 3 && strings.Count(cleaned, ",") > 1 {
			// Thousands commas: e.g. 1,250,000
			standard = strings.ReplaceAll(cleaned, ",", "")
		} else if len(afterComma) <= 2 {
			// Decimal comma: e.g. 1250000,50 or 50,5
			standard = strings.ReplaceAll(cleaned, ",", ".")
		} else {
			// Ambiguous or single thousands comma (1,250) vs 1,2500
			// In financial context, if exactly 3 digits after comma with no other separators, check standard
			standard = strings.ReplaceAll(cleaned, ",", "")
		}
	} else if hasDot {
		// Only dot present
		lastDot := strings.LastIndex(cleaned, ".")
		afterDot := cleaned[lastDot+1:]
		if len(afterDot) == 3 {
			// Indonesian thousands dots: e.g. 50.000 or 1.250.000
			standard = strings.ReplaceAll(cleaned, ".", "")
		} else {
			// Decimal dot: e.g. 50.5 or 1250.50
			standard = cleaned
		}
	} else {
		// Plain number
		standard = cleaned
	}

	// Validate with strconv.ParseFloat
	f, err := strconv.ParseFloat(standard, 64)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}

var idMonthMap = map[string]string{
	"januari":   "January",
	"februari":  "February",
	"maret":     "March",
	"april":     "April",
	"mei":       "May",
	"juni":      "June",
	"juli":      "July",
	"agustus":   "August",
	"september": "September",
	"oktober":   "October",
	"november":  "November",
	"desember":  "December",
	"jan":       "Jan",
	"feb":       "Feb",
	"mar":       "Mar",
	"apr":       "Apr",
	"jun":       "Jun",
	"jul":       "Jul",
	"agu":       "Aug",
	"ags":       "Aug",
	"sep":       "Sep",
	"okt":       "Oct",
	"nov":       "Nov",
	"des":       "Dec",
}

// ParseDate converts human dates (with optional custom format) into RFC3339 timestamp string.
func ParseDate(val, format string) (string, error) {
	cleaned := TrimWhitespace(val)
	if cleaned == "" {
		return "", errors.New("empty date")
	}

	// Normalize Indonesian month names if present
	normalized := cleaned
	for idM, enM := range idMonthMap {
		re := regexp.MustCompile(`(?i)\b` + idM + `\b`)
		normalized = re.ReplaceAllString(normalized, enM)
	}

	// If explicit format provided, translate human tokens to Go layout
	if format != "" {
		layout := translateDateFormat(format)
		t, err := time.Parse(layout, normalized)
		if err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}

	// Try common layouts
	commonLayouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02/01/2006 15:04:05",
		"02/01/2006 15:04",
		"02/01/2006",
		"02-01-2006 15:04:05",
		"02-01-2006 15:04",
		"02-01-2006",
		"02 Jan 2006 15:04:05",
		"02 Jan 2006 15:04",
		"02 Jan 2006",
		"02 January 2006 15:04:05",
		"02 January 2006 15:04",
		"02 January 2006",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
	}

	for _, layout := range commonLayouts {
		if t, err := time.Parse(layout, normalized); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}

	return "", fmt.Errorf("unable to parse date %q", val)
}

func translateDateFormat(format string) string {
	f := format
	f = strings.ReplaceAll(f, "YYYY", "2006")
	f = strings.ReplaceAll(f, "YY", "06")
	f = strings.ReplaceAll(f, "MMMM", "January")
	f = strings.ReplaceAll(f, "MMM", "Jan")
	f = strings.ReplaceAll(f, "MM", "01")
	f = strings.ReplaceAll(f, "DD", "02")
	f = strings.ReplaceAll(f, "HH", "15")
	f = strings.ReplaceAll(f, "mm", "04")
	f = strings.ReplaceAll(f, "ss", "05")
	return f
}

// TitleCase converts a string to Title Case.
func TitleCase(val string) string {
	cleaned := TrimWhitespace(val)
	if cleaned == "" {
		return ""
	}
	words := strings.Fields(strings.ToLower(cleaned))
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}
