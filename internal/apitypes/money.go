// Package apitypes provides shared, minimal wire-format value-object shapes
// reused across module DTOs per 21_project_structure.md §5.1.
//
// These types contain no domain logic and exist solely to standardize
// API boundary serialization (e.g., decimal-string monetary amounts and confidence scores).
package apitypes

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// ErrInvalidMoneyValue indicates the monetary amount string does not match numeric(18,4) format.
	ErrInvalidMoneyValue = errors.New("invalid money value: must be a decimal string with up to 4 decimal places")
	// ErrInvalidCurrency indicates an invalid currency code (must be 3 uppercase alphabetic characters).
	ErrInvalidCurrency = errors.New("invalid currency: must be a 3-letter ISO-4217 uppercase code")
)

// decimalRegex enforces numeric(18,4) format: optional negative sign, 1-18 digits, optional decimal point with 1-4 digits.
var decimalRegex = regexp.MustCompile(`^-?\d{1,18}(\.\d{1,4})?$`)

// currencyRegex enforces 3-letter uppercase ISO currency code.
var currencyRegex = regexp.MustCompile(`^[A-Z]{3}$`)

// Money represents a fixed-point monetary amount and its currency.
// Wire-format representation transmits value as a decimal string to prevent floating-point precision loss
// per 08_database_design.md §20, 09_api_specification.md §14, and 20_coding_convention.md §9.2.
type Money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// NewMoney constructs a validated Money value object.
func NewMoney(value, currency string) (Money, error) {
	m := Money{
		Value:    strings.TrimSpace(value),
		Currency: strings.ToUpper(strings.TrimSpace(currency)),
	}
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	return m, nil
}

// Validate checks whether the monetary value and currency adhere to platform specifications.
func (m Money) Validate() error {
	if m.Value == "" || !decimalRegex.MatchString(m.Value) {
		return fmt.Errorf("%w: %q", ErrInvalidMoneyValue, m.Value)
	}
	if !currencyRegex.MatchString(m.Currency) {
		return fmt.Errorf("%w: %q", ErrInvalidCurrency, m.Currency)
	}
	return nil
}

// String returns human-readable representation: "45.00 USD".
func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.Value, m.Currency)
}
