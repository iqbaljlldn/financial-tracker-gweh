package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	decimalPattern  = regexp.MustCompile(`^(\d{1,14})(?:\.(\d{1,4}))?$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Money is the immutable value object for monetary amounts (04_domain_model.md §5.1.1).
// The value is held as a normalized decimal string and is never converted through a binary float
// inside the aggregate, so it serializes as a decimal string, never a JSON float (09_api_specification.md §4.2).
type Money struct {
	value    string
	currency string
}

// NewMoney validates and normalizes a decimal-string amount.
// Amounts are strictly positive; direction (income vs expense) is carried by event_type.
func NewMoney(value, currency string) (Money, error) {
	value = strings.TrimSpace(value)
	currency = strings.TrimSpace(currency)

	if !currencyPattern.MatchString(currency) {
		return Money{}, fmt.Errorf("%w: got %q", ErrInvalidCurrency, currency)
	}
	norm, err := NormalizeDecimal(value)
	if err != nil {
		return Money{}, err
	}
	return Money{value: norm, currency: currency}, nil
}

// MoneyFromFloat converts the extractor's float64 candidate amount (the only float boundary in the
// platform) into a decimal-string Money rounded to 4 fractional digits.
func MoneyFromFloat(v float64, currency string) (Money, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Money{}, fmt.Errorf("%w: got %v", ErrInvalidAmount, v)
	}
	return NewMoney(strconv.FormatFloat(v, 'f', 4, 64), currency)
}

// NormalizeDecimal validates a decimal string and renders it with at least 2 and at most 4
// fractional digits (trailing zeros beyond the second are trimmed), e.g. "42.5" -> "42.50".
// Zero and negative values are rejected.
func NormalizeDecimal(s string) (string, error) {
	m := decimalPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", fmt.Errorf("%w: got %q", ErrInvalidAmount, s)
	}

	intPart := strings.TrimLeft(m[1], "0")
	if intPart == "" {
		intPart = "0"
	}
	frac := m[2]
	for len(frac) < 2 {
		frac += "0"
	}
	for len(frac) > 2 && strings.HasSuffix(frac, "0") {
		frac = frac[:len(frac)-1]
	}

	if intPart == "0" && strings.Trim(frac, "0") == "" {
		return "", fmt.Errorf("%w: zero is not allowed", ErrInvalidAmount)
	}
	return intPart + "." + frac, nil
}

// Value returns the normalized decimal string.
func (m Money) Value() string { return m.value }

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string { return m.currency }

// IsZero reports whether the Money has never been set.
func (m Money) IsZero() bool { return m.value == "" && m.currency == "" }

// Equal compares value and currency exactly.
func (m Money) Equal(o Money) bool { return m.value == o.value && m.currency == o.currency }

type moneyWire struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// MarshalJSON serializes the amount as {"value":"45.00","currency":"USD"}.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(moneyWire{Value: m.value, Currency: m.currency})
}

// UnmarshalJSON accepts only a decimal *string* value so a JSON float can never enter the ledger.
func (m *Money) UnmarshalJSON(data []byte) error {
	var raw struct {
		Value    json.RawMessage `json:"value"`
		Currency string          `json:"currency"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}

	var value string
	if err := json.Unmarshal(raw.Value, &value); err != nil {
		return fmt.Errorf("%w: value must be a JSON string, not a number", ErrInvalidAmount)
	}

	parsed, err := NewMoney(value, raw.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// Merchant value object (04_domain_model.md §5.1.1).
type Merchant struct {
	NormalizedName string  `json:"normalizedName"`
	RawName        string  `json:"rawName"`
	MerchantID     *string `json:"merchantId,omitempty"`
}

// DisplayName returns the normalized name, falling back to the raw name.
func (m Merchant) DisplayName() string {
	if strings.TrimSpace(m.NormalizedName) != "" {
		return m.NormalizedName
	}
	return m.RawName
}

// PaymentMethod value object (04_domain_model.md §5.1.1).
type PaymentMethod struct {
	Type string `json:"type"`
	Hint string `json:"hint"`
}
