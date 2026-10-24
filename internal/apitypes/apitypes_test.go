package apitypes_test

import (
	"encoding/json"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/apitypes"
)

func TestMoney_ValidationAndJSON(t *testing.T) {
	tests := []struct {
		name        string
		val         string
		curr        string
		wantErr     bool
		expectedStr string
	}{
		{
			name:        "valid USD amount",
			val:         "45.00",
			curr:        "USD",
			wantErr:     false,
			expectedStr: "45.00 USD",
		},
		{
			name:        "valid 4 decimal places",
			val:         "123.4567",
			curr:        "IDR",
			wantErr:     false,
			expectedStr: "123.4567 IDR",
		},
		{
			name:    "invalid decimal places (more than 4)",
			val:     "12.34567",
			curr:    "USD",
			wantErr: true,
		},
		{
			name:    "invalid non-numeric value",
			val:     "abc",
			curr:    "USD",
			wantErr: true,
		},
		{
			name:    "invalid currency length",
			val:     "10.00",
			curr:    "US",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := apitypes.NewMoney(tt.val, tt.curr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.String() != tt.expectedStr {
				t.Errorf("expected string %q, got %q", tt.expectedStr, m.String())
			}

			// Test JSON roundtrip
			bytes, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("json marshal failed: %v", err)
			}
			var decoded apitypes.Money
			if err := json.Unmarshal(bytes, &decoded); err != nil {
				t.Fatalf("json unmarshal failed: %v", err)
			}
			if decoded.Value != m.Value || decoded.Currency != m.Currency {
				t.Errorf("expected %+v, got %+v", m, decoded)
			}
		})
	}
}

func TestConfidence_ValidationAndJSON(t *testing.T) {
	t.Run("bare float serialization", func(t *testing.T) {
		c, err := apitypes.NewConfidence(0.94)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		if string(data) != "0.94" {
			t.Errorf("expected '0.94', got %q", string(data))
		}

		var decoded apitypes.Confidence
		if err := json.Unmarshal([]byte("0.94"), &decoded); err != nil {
			t.Fatalf("failed to unmarshal float: %v", err)
		}
		if decoded.Value != 0.94 {
			t.Errorf("expected 0.94, got %v", decoded.Value)
		}
	})

	t.Run("structured object serialization", func(t *testing.T) {
		perField := map[string]float64{
			"amount":   0.98,
			"merchant": 0.85,
		}
		c, err := apitypes.NewRichConfidence(0.915, "ai_weighted", perField)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("failed to marshal rich confidence: %v", err)
		}

		var decoded apitypes.Confidence
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal object: %v", err)
		}
		if decoded.Value != 0.915 || decoded.Method != "ai_weighted" || decoded.PerField["amount"] != 0.98 {
			t.Errorf("unexpected decoded object: %+v", decoded)
		}
	})

	t.Run("invalid confidence range", func(t *testing.T) {
		if _, err := apitypes.NewConfidence(1.05); err == nil {
			t.Errorf("expected error for score > 1.0, got nil")
		}
		if _, err := apitypes.NewConfidence(-0.1); err == nil {
			t.Errorf("expected error for score < 0.0, got nil")
		}
	})
}
