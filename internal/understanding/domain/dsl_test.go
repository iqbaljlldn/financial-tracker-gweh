package domain_test

import (
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestDSL_PostProcessing_TrimWhitespace(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"  hello world  ", "hello world"},
		{"hello\n\t  world\r\n", "hello world"},
		{"", ""},
		{"   ", ""},
		{"single", "single"},
	}

	for _, tc := range cases {
		out := domain.TrimWhitespace(tc.input)
		if out != tc.expected {
			t.Errorf("TrimWhitespace(%q) = %q; want %q", tc.input, out, tc.expected)
		}
	}
}

func TestDSL_PostProcessing_StripCurrencySymbol(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Rp 50.000", "50.000"},
		{"Rp. 125.000", "125.000"},
		{"IDR 1,500,000.00", "1,500,000.00"},
		{"$45.99", "45.99"},
		{"USD 120.00", "120.00"},
		{"€ 99.50", "99.50"},
		{"EUR 15.00", "15.00"},
		{"S$ 25.50", "25.50"},
		{"SGD 10.00", "10.00"},
		{"¥ 5000", "5000"},
	}

	for _, tc := range cases {
		out := domain.StripCurrencySymbol(tc.input)
		if out != tc.expected {
			t.Errorf("StripCurrencySymbol(%q) = %q; want %q", tc.input, out, tc.expected)
		}
	}
}

func TestDSL_PostProcessing_ParseDecimal(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
		hasErr   bool
	}{
		{"Indonesian dot thousands with comma decimal", "1.250.000,50", "1250000.5", false},
		{"Indonesian dot thousands integer", "1.250.000", "1250000", false},
		{"Indonesian dot thousands smaller", "50.000", "50000", false},
		{"US comma thousands with dot decimal", "1,250,000.50", "1250000.5", false},
		{"US comma thousands integer", "1,250,000", "1250000", false},
		{"Simple decimal", "50.5", "50.5", false},
		{"Simple integer", "50000", "50000", false},
		{"Negative amount", "-15000", "-15000", false},
		{"Negative with dot thousands", "-15.000,50", "-15000.5", false},
		{"Empty string", "", "", true},
		{"Malformed text", "invalid-number", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := domain.ParseDecimal(tc.input)
			if tc.hasErr {
				if err == nil {
					t.Fatalf("ParseDecimal(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDecimal(%q) unexpected error: %v", tc.input, err)
			}
			if out != tc.expected {
				t.Errorf("ParseDecimal(%q) = %q; want %q", tc.input, out, tc.expected)
			}
		})
	}
}

func TestDSL_PostProcessing_ParseDate(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		format   string
		expected string
		hasErr   bool
	}{
		{
			name:     "Indonesian date with full month",
			input:    "25 Oktober 2026 14:30:00",
			format:   "DD MMMM YYYY HH:mm:ss",
			expected: "2026-10-25T14:30:00Z",
			hasErr:   false,
		},
		{
			name:     "Indonesian date short month",
			input:    "25 Okt 2026",
			format:   "DD MMM YYYY",
			expected: "2026-10-25T00:00:00Z",
			hasErr:   false,
		},
		{
			name:     "Slash date with time",
			input:    "25/10/2026 15:45",
			format:   "DD/MM/YYYY HH:mm",
			expected: "2026-10-25T15:45:00Z",
			hasErr:   false,
		},
		{
			name:     "ISO 8601 auto-detected",
			input:    "2026-10-25T10:15:30Z",
			format:   "",
			expected: "2026-10-25T10:15:30Z",
			hasErr:   false,
		},
		{
			name:     "Malformed date",
			input:    "bukan tanggal",
			format:   "",
			expected: "",
			hasErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := domain.ParseDate(tc.input, tc.format)
			if tc.hasErr {
				if err == nil {
					t.Fatalf("ParseDate(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDate(%q) unexpected error: %v", tc.input, err)
			}
			if out != tc.expected {
				t.Errorf("ParseDate(%q) = %q; want %q", tc.input, out, tc.expected)
			}
		})
	}
}

func TestDSL_PostProcessing_TitleCase(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"pt grab teknologi indonesia", "Pt Grab Teknologi Indonesia"},
		{"INDOMARET POINT", "Indomaret Point"},
		{"kopi kenangan mantan", "Kopi Kenangan Mantan"},
		{"", ""},
	}

	for _, tc := range cases {
		out := domain.TitleCase(tc.input)
		if out != tc.expected {
			t.Errorf("TitleCase(%q) = %q; want %q", tc.input, out, tc.expected)
		}
	}
}

func TestDSL_ApplyPostProcessing_ClosedEnumeratedSetConstraint(t *testing.T) {
	// [GATE]: Arbitrary code execution prevention. Only closed enumerated set allowed.
	invalidSteps := []string{"trimWhitespace", "evalPythonScript:import os; os.system('ls')"}
	_, err := domain.ApplyPostProcessing("test", invalidSteps)
	if err == nil {
		t.Fatalf("expected error on unknown postProcessing step, got nil")
	}

	validSteps := []string{
		domain.PostProcessingStripCurrencySymbol,
		domain.PostProcessingTrimWhitespace,
		domain.PostProcessingParseDecimal,
	}
	out, err := domain.ApplyPostProcessing("  Rp 1.500.000,00  ", validSteps)
	if err != nil {
		t.Fatalf("unexpected error for valid pipeline: %v", err)
	}
	if out != "1500000" {
		t.Errorf("expected 1500000, got: %q", out)
	}
}
