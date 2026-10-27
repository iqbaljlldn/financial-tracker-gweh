# Sprint 9 — Engagement: WhatsApp Notifications (Platform-Agnostic)

**Theme:** Build the `engagement` module with a **platform-agnostic messaging architecture** (`MessagingClient` port) supporting **Fonnte** (primary/recommended for local/Indonesian gateway), **Meta WhatsApp Cloud API**, and **Mock** providers. Includes message template rendering, composition from Analytics read models, constrained-intent reply parsing, and two-tier delivery confirmation.

**Primary documents:** `15_notification_system.md` (all)

**Entry criteria:** Sprint 8 complete (Analytics read models available); Sprint 7 complete (`BudgetThresholdCrossed` available).

---

## Checklist

### WhatsApp Provider Setup (Agnostic: Fonnte / Meta / Mock) **[GATE]**
- [ ] Select and configure active provider in config (`engagement.provider: "fonnte"` | `"meta"` | `"mock"`)
- [ ] If using **Fonnte**: Obtain API token from Fonnte dashboard, configure device pairing and webhook URL
- [ ] If using **Meta**: Provision WhatsApp Business API account, configure phone number ID, and register templates (`daily_summary_v1`, `weekly_summary_v1`, `monthly_report_v1`, `budget_alert_v1`, `review_request_v1`)
- [ ] If using **Mock**: Ensure test fixtures and in-memory mock client are ready for local zero-dependency testing

### Engagement Module — Domain Layer
- [ ] `domain/notification_job.go` — `NotificationJob` aggregate
- [ ] `domain/repository.go` — `NotificationJobRepository`, `NotificationPreferenceRepository`
- [ ] `domain/messaging_client.go` — `MessagingClient` port interface (`SendMessage`, `ProviderName`)

### Preference Gate
- [ ] `notification_preferences` check before any composition work begins
- [ ] Opted-out → clean no-op, distinct from a delivery failure (for accurate dashboard reporting) **[GATE]**

### Content Composer & Template Renderer
- [ ] Reads **exclusively** from Analytics read models, never `financialcore` directly **[GATE]**
- [ ] Template selection & rendering engine:
  - Formats rich WhatsApp markdown (`*bold*`, `_italic_`, line breaks, currency formatting) for Fonnte/plain text
  - Formats parameterized placeholder payload for Meta Cloud API
- [ ] Hard failure (not silent fallback) if a `notification_type` has no template/renderer configured

### WhatsApp Delivery Client (Agnostic Adapters)
- [ ] `adapter/external/messaging/factory.go` — provider factory instantiating configured client
- [ ] `adapter/external/messaging/fonnte/client.go` — Fonnte API (`POST https://api.fonnte.com/send`)
- [ ] `adapter/external/messaging/meta/client.go` — Meta WhatsApp Cloud API
- [ ] `adapter/external/messaging/mock/client.go` — In-memory test client with message recording
- [ ] Rate-limit-aware sending against provider rate limits
- [ ] Retry classification: transient vs. permanent (blocked number → terminal, disable preferences)

### Constrained-Intent Reply Parsing (H4 Resolution) **[GATE — this touches the ledger, get it right]**
- [ ] Inbound reply webhook handling supporting both Fonnte and Meta payload shapes:
  - Structured button reply (`Confirm`/`Edit`) — **no NLU**
  - Fallback: strict whitelisted text pattern matching (`^Y$`/`^N$`, `^YA$`/`^TIDAK$`)
- [ ] `[Edit]` flow: bounded follow-up (amount/merchant/category button or key choice), never open text
- [ ] Unrecognized reply → clarifying re-prompt, never silently misinterpreted

### Two-Tier Delivery Confirmation
- [ ] `POST /webhooks/whatsapp/delivery-status` handler (or provider-specific webhook routers)
- [ ] Tracks `provider_accepted` / `delivered` / `read` separately based on provider callbacks
- [ ] `read` tier never used as a functional gate anywhere — observability only **[GATE]**

### Scheduled Notifications
- [ ] Daily/weekly/monthly summary scheduling, timezone-aware per user
- [ ] `ComposeBudgetAlertNotification` wired to Sprint 7's `BudgetThresholdCrossed`
- [ ] `ComposeReviewRequestNotification` wired to Sprint 6's `FinancialEventPendingReview`

### Quality & Rate Governance
- [ ] `engagement:deliver` queue rate-limited to configured provider tier
- [ ] Metrics: `engagement_notifications_composed_total`, `engagement_delivery_outcome_total`, `engagement_whatsapp_block_rate`, `engagement_reply_parse_outcome_total` (`17_observability.md` §4.6)

### Frontend (Parallel Track)
- [ ] Notification preferences screen: toggle each `notification_type` independently (`GET/PATCH /notifications/preferences`)
- [ ] Confirmation that a toggle off for one type never affects others (visible in the UI state, matching the backend guarantee)
- [ ] (Optional, [DEFER-OK]) In-app notification history view, mirroring what was sent via WhatsApp, for users who want a web-based record

### Testing
- [ ] Unit & contract tests for `FonnteClient`, `MetaClient`, and `MockClient`
- [ ] Integration test: Diagram 7 — delivery retry on transient failure, eventual success
- [ ] Integration test: button-reply confirmation flow end to end (finalizes `pending_review` event in ledger)
- [ ] Integration test: text-pattern fallback flow (`Y`/`N`)
- [ ] Fixture-based webhook parsing test for both Fonnte and Meta payloads

---

## Exit Criteria

- [ ] A real (or sandbox/Fonnte/Mock) WhatsApp number receives a correctly composed daily summary
- [ ] Confirming a pending-review event via button/text reply correctly finalizes it in the ledger
- [ ] Switching provider between Fonnte, Meta, or Mock requires only config changes without touching domain code
- [ ] Opting out of one notification type verifiably leaves others unaffected
