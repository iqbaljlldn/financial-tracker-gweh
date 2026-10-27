package fonnte

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

func TestFonnteSendResponse_UnmarshalVariations(t *testing.T) {
	tests := []struct {
		name       string
		jsonBody   string
		expectedID string
		status     bool
	}{
		{
			name: "real fonnte response with array of numbers",
			jsonBody: `{
				"detail": "success! message in queue",
				"id": [188762821],
				"process": "pending",
				"quota": {
					"62889751449074": {
						"details": "deduced from total quota",
						"quota": 999,
						"remaining": 998,
						"used": 1
					}
				},
				"requestid": 1067543652,
				"status": true,
				"target": ["6285183071309"]
			}`,
			expectedID: "188762821",
			status:     true,
		},
		{
			name:       "array of strings",
			jsonBody:   `{"status": true, "id": ["msg_string_123"], "detail": "success"}`,
			expectedID: "msg_string_123",
			status:     true,
		},
		{
			name:       "single number ID",
			jsonBody:   `{"status": true, "id": 99887766, "detail": "success"}`,
			expectedID: "99887766",
			status:     true,
		},
		{
			name:       "single string ID",
			jsonBody:   `{"status": true, "id": "single_id_val", "detail": "success"}`,
			expectedID: "single_id_val",
			status:     true,
		},
		{
			name:       "empty ID array with requestid fallback",
			jsonBody:   `{"status": true, "id": [], "requestid": 1067543652}`,
			expectedID: "1067543652",
			status:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var resp fonnteSendResponse
			if err := json.Unmarshal([]byte(tc.jsonBody), &resp); err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}

			if resp.Status != tc.status {
				t.Errorf("expected status %v, got %v", tc.status, resp.Status)
			}

			msgID := ""
			if len(resp.ID) > 0 {
				msgID = resp.ID[0]
			} else if len(resp.RequestID) > 0 {
				msgID = string(resp.RequestID)
			}

			if msgID != tc.expectedID {
				t.Errorf("expected msgID %q, got %q", tc.expectedID, msgID)
			}
		})
	}
}

func TestClient_SendMessage_SuccessRealPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "valid_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"detail": "success! message in queue",
			"id": [188762821],
			"process": "pending",
			"quota": {"62889751449074": {"details": "deduced from total quota", "quota": 999, "remaining": 998, "used": 1}},
			"requestid": 1067543652,
			"status": true,
			"target": ["6285183071309"]
		}`))
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:  server.URL,
		APIToken: "valid_token",
	})

	res, err := client.SendMessage(context.Background(), domain.OutboundMessage{
		RecipientPhone: "085183071309",
		RenderedText:   "Review request: Starbucks Rp 50.000",
	})

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if res.ProviderMessageID != "188762821" {
		t.Errorf("expected ProviderMessageID '188762821', got %q", res.ProviderMessageID)
	}
	if res.Provider != "fonnte" {
		t.Errorf("expected provider 'fonnte', got %q", res.Provider)
	}
}
