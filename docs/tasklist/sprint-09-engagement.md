# Sprint 9 — Engagement: WhatsApp Notifications

**Theme:** Build the `engagement` module — WhatsApp Business API integration, approved message templates, composition from Analytics read models, constrained-intent reply parsing, and two-tier delivery confirmation.

**Primary documents:** `15_notification_system.md` (all)

**Entry criteria:** Sprint 8 complete (Analytics read models available); Sprint 7 complete (`BudgetThresholdCrossed` available).

---

## Checklist

### WhatsApp Business Account Setup (Do This First — Has Lead Time) **[GATE — start this before any code]**
- [ ] WhatsApp Business API account provisioned
- [ ] Submit all MVP templates for approval **now** — approval can take hours to days (`15_notification_system.md` §4.3): `daily_summary_v1`, `weekly_summary_v1`, `monthly_report_v1`, `budget_alert_v1`, `review_request_v1`
- [ ] Confirm `UTILITY` category classification for all templates

### Engagement Module — Domain Layer
- [ ] `domain/notification_job.go` — `NotificationJob` aggregate
- [ ] `domain/repository.go`

### Preference Gate
- [ ] `notification_preferences` check before any composition work begins
- [ ] Opted-out → clean no-op, distinct from a delivery failure (for accurate dashboard reporting) **[GATE]**

### Content Composer
- [ ] Reads **exclusively** from Analytics read models, never `financialcore` directly **[GATE]**
- [ ] Template selection logic: `notification_type` → approved template ID mapping (Viper config)
- [ ] Placeholder value mapping with locale-aware currency/number formatting
- [ ] Hard failure (not silent fallback) if a `notification_type` has no approved template mapped

### WhatsApp Delivery Client
- [ ] `adapter/external/whatsappclient.go`
- [ ] Rate-limit-aware sending against the current messaging tier
- [ ] Retry classification: transient vs. permanent (blocked number → terminal, disable preferences)

### Constrained-Intent Reply Parsing (H4 Resolution) **[GATE — this touches the ledger, get it right]**
- [ ] Interactive button reply handling (`Confirm`/`Edit`) via structured `button_reply` webhook payload — **no NLU**
- [ ] `[Edit]` flow: bounded follow-up (amount/merchant/category button choice), never open text
- [ ] Fallback: strict whitelisted text pattern matching (`^Y$`/`^N$`) for clients without interactive buttons
- [ ] Unrecognized reply → clarifying re-prompt, never silently misinterpreted

### Two-Tier Delivery Confirmation
- [ ] `POST /webhooks/whatsapp/delivery-status` handler, signature-verified
- [ ] Tracks `provider_accepted` / `delivered` / `read` separately
- [ ] `read` tier never used as a functional gate anywhere — observability only **[GATE]**

### Scheduled Notifications
- [ ] Daily/weekly/monthly summary scheduling, timezone-aware per user
- [ ] `ComposeBudgetAlertNotification` wired to Sprint 7's `BudgetThresholdCrossed`
- [ ] `ComposeReviewRequestNotification` wired to Sprint 6's `FinancialEventPendingReview`

### Quality & Rate Governance
- [ ] `engagement:deliver` queue rate-limited to current messaging tier (`06_architecture.md` §6.3)
- [ ] Metrics: `engagement_notifications_composed_total`, `engagement_delivery_outcome_total`, `engagement_whatsapp_block_rate`, `engagement_reply_parse_outcome_total` (`17_observability.md` §4.6)

### Frontend (Parallel Track)
- [ ] Notification preferences screen: toggle each `notification_type` independently (`GET/PATCH /notifications/preferences`)
- [ ] Confirmation that a toggle off for one type never affects others (visible in the UI state, matching the backend guarantee)
- [ ] (Optional, [DEFER-OK]) In-app notification history view, mirroring what was sent via WhatsApp, for users who want a web-based record

### Testing
- [ ] Integration test: Diagram 7 — delivery retry on transient failure, eventual success
- [ ] Integration test: button-reply confirmation flow end to end
- [ ] Integration test: text-pattern fallback flow
- [ ] Fixture-based contract test against recorded WhatsApp webhook payload shapes

---

## Exit Criteria

- [ ] A real (or sandbox) WhatsApp number receives a correctly composed daily summary
- [ ] Confirming a pending-review event via button reply correctly finalizes it in the ledger
- [ ] All MVP templates are approved and mapped — no `notification_type` is unmapped
- [ ] Opting out of one notification type verifiably leaves others unaffected
