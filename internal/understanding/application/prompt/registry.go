package prompt

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrPromptVersionNotFound = errors.New("prompt version not found in registry")
	ErrPromptVersionExists   = errors.New("prompt version already registered and immutable")
)

// Registry manages immutable prompt versions and config-driven activation/rollback (11_ai_blueprint.md §4.2).
type Registry struct {
	mu            sync.RWMutex
	versions      map[string]*PromptVersion
	activeVersion string
}

// NewRegistry initializes a prompt registry with baseline active version "v1".
func NewRegistry() *Registry {
	r := &Registry{
		versions:      make(map[string]*PromptVersion),
		activeVersion: "v1",
	}

	_ = r.Register(GetV1PromptVersion())
	return r
}

// Register registers a new immutable prompt version. Fails if the version already exists [GATE].
func (r *Registry) Register(pv *PromptVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.versions[pv.VersionID]; exists {
		return fmt.Errorf("%w: %s", ErrPromptVersionExists, pv.VersionID)
	}
	r.versions[pv.VersionID] = pv
	return nil
}

// Get retrieves a prompt version by ID.
func (r *Registry) Get(versionID string) (*PromptVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pv, exists := r.versions[versionID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrPromptVersionNotFound, versionID)
	}
	return pv, nil
}

// GetActive retrieves the currently active prompt version.
func (r *Registry) GetActive() *PromptVersion {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.versions[r.activeVersion]
}

// ActivateVersion activates a registered prompt version (config-driven rollback/promotion per 11_ai_blueprint.md §4.2).
func (r *Registry) ActivateVersion(versionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.versions[versionID]; !exists {
		return fmt.Errorf("%w: %s", ErrPromptVersionNotFound, versionID)
	}
	r.activeVersion = versionID
	return nil
}

// ActiveVersionID returns the current active prompt version ID.
func (r *Registry) ActiveVersionID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.activeVersion
}

// GetV1PromptVersion returns the standard production v1 prompt configuration.
func GetV1PromptVersion() *PromptVersion {
	return &PromptVersion{
		VersionID: "v1",
		SystemInstruction: `You are an expert financial event extraction assistant for a Personal Finance Assistant Platform.
Extract transaction details from the provided email with high precision.

CONFIDENCE RUBRIC:
- 1.0: Explicitly and unambiguously stated in the email text.
- 0.7 - 0.9: Strongly implied but required minor inference (e.g. standard merchant name normalization).
- 0.4 - 0.6: Inferred from indirect context.
- Below 0.4: Low-confidence guess or ambiguous data.

RULES:
- Return ONLY valid JSON matching the exact schema. Do not output markdown fences or explanatory text.
- If a field is not present or cannot be determined with confidence, use null or omit it. Never fabricate data.
- Event types allowed: "Income", "Salary", "Dividend", "Expense", "Transfer", "Refund", "Cashback", "SubscriptionRenewal", "FailedPayment", "Investment", "Loan", "CreditCardBill", "Tax", "Donation", "Others".
- Explain any uncertainty or ambiguity in the "extractionNotes" field.`,

		SchemaDefinition: `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "AMZN Mktp US", "normalizedName": "Amazon", "confidence": 0.85 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.70 },
  "categoryHint": { "value": "Shopping", "confidence": 0.60 },
  "typeSpecificPayload": {},
  "extractionNotes": "Clear transaction total on receipt."
}`,

		FewShotExamples: []FewShotExample{
			{
				EmailContent: `Order Confirmation: Your Amazon.com order #112-9871234
Order Total: $45.00
Payment method: Visa ending in 4821
Date: Aug 3, 2026 14:22 UTC
Shipped to: Seattle, WA`,
				TargetJSON: `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.98 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Amazon.com", "normalizedName": "Amazon", "confidence": 0.95 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.90 },
  "categoryHint": { "value": "Shopping", "confidence": 0.80 },
  "typeSpecificPayload": {},
  "extractionNotes": "Amazon order confirmation with explicit card and amount."
}`,
			},
			{
				EmailContent: `Bukti Transaksi GoPay
Nomor Pesanan: GP-99881122
Total Pembayaran: Rp 25.000
Metode Pembayaran: Saldo GoPay
Waktu: 2026-08-04 10:15:00 WIB
Merchant: Kopi Kenangan`,
				TargetJSON: `{
  "eventType": "Expense",
  "amount": { "value": "25000", "currency": "IDR", "confidence": 0.99 },
  "occurredAt": { "value": "2026-08-04T03:15:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Kopi Kenangan", "normalizedName": "Kopi Kenangan", "confidence": 0.95 },
  "paymentMethod": { "type": "e_wallet", "hint": "GoPay", "confidence": 0.95 },
  "categoryHint": { "value": "Food & Beverage", "confidence": 0.85 },
  "typeSpecificPayload": {},
  "extractionNotes": "Indonesian e-wallet transaction receipt."
}`,
			},
		},
	}
}
