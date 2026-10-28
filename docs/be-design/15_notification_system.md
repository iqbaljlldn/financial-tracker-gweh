# Notification System
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `06_architecture.md`, `07_sequence_diagrams.md`, `08_database_design.md`, `09_api_specification.md`, `10_backend_blueprint.md`
**Feeds Into:** `16_security.md`, `17_observability.md`, `18_testing_strategy.md`

---

## 1. Purpose

This document specifies the **Engagement module** in full: WhatsApp Business API integration mechanics, message template management (resolving Hotspot H7's pre-approval constraint), the constrained-intent reply-parsing design for pending-review confirmations (resolving Hotspot H4), and the two-tier delivery-confirmation model first introduced in `07_sequence_diagrams.md` Diagram 7.

This is the module responsible for the Vision's "WhatsApp-native delivery" differentiator (`01_product_vision.md` Section 6.1) — it must be reliable, respect user opt-outs precisely, and work within WhatsApp Business API's real operational constraints, which are considerably more restrictive than a typical push-notification API.

## 2. Scope

### 2.1 In Scope
- WhatsApp Business API integration model (session vs. template messaging).
- Message template catalog and the approval-request process (H7 resolution).
- Notification composition pipeline (from Analytics read models to rendered message).
- The constrained-intent reply-parsing design for review confirmations (H4 resolution).
- Two-tier delivery confirmation (provider-accepted vs. user-delivered vs. user-read).
- Opt-out and preference enforcement mechanics.
- Rate-limit and quality-rating governance (WhatsApp Business API imposes both).

### 2.2 Out of Scope
- Analytics computation itself (consumed, not computed, here) → `02_product_requirement_document.md`, `08_database_design.md` Section 11.
- General Outbox/Inbox/retry infrastructure (reused, not redefined, here) → `06_architecture.md` Section 6.
- Budget threshold evaluation logic (only its resulting alert is composed/delivered here) → `04_domain_model.md` Section 9.

---

## 3. Architecture: Engagement Module Pipeline

```
Trigger (scheduled: daily/weekly/monthly; or event-driven: BudgetThresholdCrossed,
         FinancialEventPendingReview)
        │
        ▼
┌─────────────────────────────┐
│  Preference Gate                │  ← check notification_preferences;
│                               │    skip if user has opted out of
│                               │    this notification_type
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Content Composer               │  ← reads Analytics read models
│                               │    (never raw financialcore data),
│                               │    selects the approved WhatsApp
│                               │    template matching this content shape
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Template Renderer              │  ← fills template placeholders with
│                               │    composed values (Markdown/HSM)
└──────────────┬───────────────┘
               ▼
┌─────────────────────────────┐
│  Messaging Client (Port)        │  ← Agnostic Interface:
│  [Fonnte | Meta | Mock]         │    handles rate limits/retries
└──────────────┬───────────────┘
               ▼
     [provider-accepted] ──▶ [delivered] ──▶ [read] (webhook-driven,
                                                       two-tier per Section 7)
```

---

## 4. Platform-Agnostic Messaging Architecture & Provider Integration

### 4.1 Hexagonal Design: The `MessagingClient` Port

To prevent vendor lock-in (`01_product_vision.md` Section 6.2) and eliminate operational blockers like Meta WhatsApp Business account verification delays, the Engagement module employs a strict **Hexagonal Architecture (Ports and Adapters)** pattern.

The domain defines a provider-agnostic port:

```go
type OutboundMessage struct {
    RecipientPhone   string            // E.164 formatted or local Indonesian format (e.g., 08xxx / 628xxx)
    NotificationType NotificationType  // daily_summary, budget_alert, review_request, etc.
    TemplateID       string            // e.g., daily_summary_v1
    Parameters       map[string]string // placeholder variables: totalSpendYesterday, topCategory, etc.
    RenderedText     string            // WhatsApp markdown rendered text (for Fonnte / plaintext)
    Buttons          []ActionButton    // Quick action buttons (Confirm / Edit)
}

type ActionButton struct {
    ID    string // e.g. "confirm:event_uuid"
    Label string // e.g. "Confirm"
}

type SendResult struct {
    ProviderMessageID string
    Status            DeliveryStatus   // accepted, delivered, failed
    Provider          string           // "fonnte", "meta", "mock"
}

type MessagingClient interface {
    SendMessage(ctx context.Context, msg OutboundMessage) (*SendResult, error)
    ProviderName() string
}
```

The active provider is chosen dynamically at startup via configuration:
```yaml
engagement:
  provider: "fonnte" # "fonnte" | "meta" | "mock"
  fonnte:
    base_url: "https://api.fonnte.com"
    api_token: "${FONNTE_API_TOKEN}"
    country_code: "62"
  meta:
    phone_number_id: "${META_PHONE_NUMBER_ID}"
    access_token: "${META_ACCESS_TOKEN}"
    app_secret: "${META_APP_SECRET}"
    webhook_verify_token: "${META_WEBHOOK_VERIFY_TOKEN}"
```

### 4.2 Supported Providers

#### 1. Fonnte (`fonnte`) — Primary / Recommended
- **Overview:** Popular WhatsApp API Gateway in Indonesia, ideal for indie developers and production deployments without cumbersome enterprise business verification.
- **Dispatch Endpoint:** `POST https://api.fonnte.com/send`
- **Authentication:** `Authorization: <token>` HTTP header.
- **Payload Structure:**
  ```json
  {
    "target": "628123456789",
    "message": "*Ringkasan Harian Keuangan*\n\nTotal pengeluaran kemarin: *Rp 150.000*...",
    "button": "[{\"id\":\"confirm:uuid\",\"text\":\"Confirm\"},{\"id\":\"edit:uuid\",\"text\":\"Edit\"}]",
    "countryCode": "62"
  }
  ```
- **Key Operational Advantage:** Does **not** require waiting for Meta's manual template approval process (resolves Hotspot H7 operational friction immediately). Messages are formatted locally with WhatsApp native markdown (`*bold*`, `_italic_`, emojis, lists).
- **Inbound Webhook (`POST /webhooks/whatsapp/fonnte`):**
  Fonnte pushes incoming replies directly:
  ```json
  {
    "sender": "628123456789",
    "message": "Confirm",
    "button": "{\"id\":\"confirm:uuid\"}",
    "id": "msg_xyz123"
  }
  ```

#### 2. Meta WhatsApp Cloud API (`meta`) — Enterprise Official
- **Overview:** Official Meta Graph API (`https://graph.facebook.com/v20.0/{phone_number_id}/messages`).
- **Authentication:** Bearer token + Webhook signature verification (`X-Hub-Signature-256`).
- **Constraint:** Requires pre-approved HSM templates for proactive business-initiated messages (`UTILITY` category), and adherence to 24-hour customer service windows for free-form replies.

#### 3. Mock Client (`mock`) — Testing & CI
- **Overview:** Deterministic in-memory client that records sent messages and allows simulating delivery status and reply webhooks without external network dependencies.

### 4.3 Template Catalog & Markdown Rendering

The platform maintains a standardized catalog mapping `notification_type` to message shapes:

| Template ID | `notification_type` | Placeholder Slots | Rich Markdown Representation |
|---|---|---|---|
| `daily_summary_v1` | `daily_summary` | `totalSpendYesterday`, `topCategory`, `eventCount` | `📊 *Ringkasan Harian*\nPengeluaran kemarin: *{{totalSpendYesterday}}*...` |
| `weekly_summary_v1` | `weekly_summary` | `totalIncome`, `totalExpense`, `topCategory`, `topMerchant` | `📈 *Laporan Mingguan*\nPemasukan: *{{totalIncome}}*\nPengeluaran: *{{totalExpense}}*...` |
| `monthly_report_v1` | `monthly_report` | `netFlow`, `budgetPerformancePercent`, `comparisonToLastMonth` | `📅 *Evaluasi Bulanan*\nNet Flow: *{{netFlow}}*...` |
| `budget_alert_v1` | `budget_alert` | `categoryOrGlobal`, `percentUsed`, `thresholdAmount` | `⚠️ *Peringatan Budget!*\nKategori: *{{categoryOrGlobal}}* mencapai *{{percentUsed}}%*...` |
| `review_request_v1` | `review_request` | `amount`, `merchant`, `suggestedCategory` | `🔍 *Verifikasi Transaksi*\nNominal: *{{amount}}*\nMerchant: *{{merchant}}*\nKetik *Y* untuk Konfirmasi, *N* untuk Edit` |

- When using **Fonnte** or **Mock**, the `TemplateRenderer` executes Go templates with rich WhatsApp formatting directly.
- When using **Meta**, parameters are packaged as component parameters for Meta's Cloud API endpoint.

---

## 5. Content Composition Pipeline

### 5.1 Composer Responsibilities

Per `10_backend_blueprint.md` Section 4's `ComposeNotification` command handler:
1. Read the relevant Analytics read model(s) (`08_database_design.md` Section 11) for the triggering period/event — **never** `financialcore` tables directly, preserving the CQRS boundary even within a notification-composition context.
2. Map the read-model values onto the selected template's placeholder slots (Section 4.2's table), applying locale-aware number/currency formatting.
3. Persist the composed `notification_jobs` row (`08_database_design.md` Section 12.1) with the rendered `composed_payload`, within the same transaction as the triggering Outbox event's consumption (standard Inbox-guarded policy handler pattern, per `06_architecture.md` Section 6.2/`10_backend_blueprint.md` Section 7).

### 5.2 Preference Gate

Before composition even begins, the `notification_preferences` table (`08_database_design.md` Section 12.2) is checked; an opted-out `notification_type` results in the policy handler completing as a no-op (not an error, not a silently-dropped-but-logged-as-failed job) — this must be a clean, intentional short-circuit, distinct from a delivery failure, for accurate Administration reporting (a low delivery-success rate caused by legitimate opt-outs must never be conflated with an actual reliability problem in dashboards, per `17_observability.md`'s metric design).

---

## 6. Resolving H4: Constrained-Intent Reply Parsing for Review Confirmation

Per `05_event_storming.md`'s H4 and `07_sequence_diagrams.md` Diagram 4, confirming a `pending_review` event via WhatsApp reply must not require a second, harder AI-parsing problem layered on top of the original (already-uncertain) extraction.

### 6.1 Design: Interactive Buttons as the Primary Mechanism

The `review_request_v1` template (Section 4.2) uses **WhatsApp's native interactive message features** (Quick Reply Buttons / List Messages, supported by the Business API) rather than requesting free-text replies:
- `[Confirm]` button → sends a well-defined, structured `button_reply` webhook payload back to the platform, with a fixed `id` the platform itself assigned when composing the message (e.g., `confirm:financial_event_id=e7a9...`) — **no NLU/parsing required at all**; this is a structured protocol interaction, not text interpretation.
- `[Edit]` button → triggers a short, **explicitly bounded** follow-up interaction: the platform sends a session-window follow-up message (permitted, since the user just engaged) asking specifically "What would you like to correct? Reply with: amount / merchant / category," presenting the response as another button-based choice, not open text, keeping every step of the correction flow structured.

### 6.2 Fallback: Bounded Text Pattern-Matching

For WhatsApp clients/scenarios where interactive buttons aren't rendered (rare, but the platform must degrade gracefully — e.g., very old client versions), the template additionally instructs a plain-text reply convention (`"Reply Y to confirm, or N to edit"`), and the reply-webhook handler applies **strict, whitelisted pattern matching** (`^Y$`, `^N$`, case-insensitive, tolerant of surrounding whitespace only — not a general NLU classifier) before falling through to a "sorry, I didn't understand — please tap the buttons above" clarification message. This fallback deliberately trades a small amount of flexibility for zero ambiguity, consistent with `13_email_parser.md`'s "deterministic before probabilistic" philosophy applied here to reply handling.

### 6.3 Why This Fully Resolves H4

- No AI/LLM call is ever needed to interpret a confirmation reply — the entire flow is either a structured button-interaction protocol or a tightly-whitelisted pattern match.
- The `[Edit]` path's follow-up is itself structured (another button choice, not open text) — even the "correction" path never asks the user to freely describe what's wrong in prose that would then need parsing.
- Any reply that doesn't match an expected structured or whitelisted-text pattern receives a clarifying re-prompt rather than being silently misinterpreted or discarded — protecting against the failure mode where a user's ordinary conversational reply ("yeah that's right") would otherwise need to be correctly interpreted by fragile pattern-matching.

---

## 7. Two-Tier Delivery Confirmation

Per `07_sequence_diagrams.md` Diagram 7, the platform distinguishes and separately tracks:

| Tier | Meaning | Data Source |
|---|---|---|
| **Provider-accepted** | WhatsApp Business API returned `200 OK` for the send request, meaning the message entered WhatsApp's own delivery pipeline | Synchronous response to `DeliverViaWhatsApp` |
| **Delivered** | WhatsApp confirms the message reached the recipient's device | `/webhooks/whatsapp/delivery-status` webhook, `status: "delivered"` |
| **Read** | The recipient opened/read the message (if read receipts are enabled on their end — not guaranteed, since users can disable read receipts) | Same webhook, `status: "read"` |

All three states are recorded on `notification_jobs` (extending `08_database_design.md` Section 12.1's `delivery_confirmed_at` concept to explicitly capture which tier was reached, plus a `read_at` nullable timestamp). **"Read" is explicitly best-effort and never used as a business-logic gate** (e.g., the system never withholds a subsequent notification pending a "read" confirmation of a prior one) — it exists purely as an Administration-visible engagement metric (FR-ADMIN-01-adjacent observability), never a functional dependency, precisely because it's not guaranteed to be available for every user.

---

## 8. Opt-Out & Preference Enforcement

- Per FR-NOTIF-05: opting out of one `notification_type` must never affect others — enforced structurally by the Preference Gate (Section 5.2) checking only the single relevant boolean flag per composition, never a global "notifications enabled" switch.
- WhatsApp itself provides a platform-level opt-out mechanism (the end user blocking the business number or replying `STOP`, which WhatsApp handles at the platform level) — the Engagement module must detect and respect this via delivery-failure classification (a `blocked`-type failure response from the API is treated as **terminal**, not transient, and should trigger disabling all `notification_preferences` for that user with an in-app indication, rather than endlessly retrying against a user who has blocked the business number).

---

## 9. Rate-Limit & Quality-Rating Governance

WhatsApp Business API imposes:
- **Messaging tier limits** (a business account's daily proactive-messaging volume cap, which scales up over time based on account "quality rating" and usage history — not a fixed number this document hardcodes, since it's provider-managed and grows with good standing).
- **Quality rating impact:** A business account's ability to send messages can be throttled or suspended if too many recipients block the number or report it — meaning **notification relevance and opt-out respect are not just UX concerns but direct platform-availability risks** for this entire delivery channel.

**Design implication:** The `engagement:deliver` queue's rate-limited concurrency (`06_architecture.md` Section 6.3) must be configured against the platform's **currently-granted** messaging tier (tracked, not assumed fixed), and Administration monitoring (`17_observability.md`) must track the account's quality-rating-relevant signals (block rate, opt-out rate) as a first-class operational metric — a degrading quality rating is a leading indicator that must trigger a product/content review (e.g., is the daily summary too frequent/low-value for some users?), not merely an engineering scaling concern.

---

## 10. Detailed Explanation: Why Analytics Read Models, Never Raw Ledger Data, Feed Composition

Section 5.1 mandates that the Content Composer reads exclusively from Analytics read models. This is worth justifying explicitly because it might seem like unnecessary indirection for, e.g., a simple daily summary that could plausibly just `SELECT SUM(amount) FROM financial_events WHERE ...` directly:

1. **Consistency of aggregation logic:** If Engagement computed its own sums independently of Analytics, the platform would risk the daily WhatsApp summary and the in-app Analytics dashboard showing **different numbers for the same period** due to subtly different query logic (e.g., different currency-handling, different `archived`-status filtering) — a direct, visible trust-breaking bug for a financial product. Sourcing from the same read models guarantees consistency by construction.
2. **Respecting the CQRS boundary's purpose:** `06_architecture.md` Section 7 established Analytics as the sole computed-aggregation layer specifically to keep `financialcore` free of read-optimization concerns. Allowing Engagement to bypass this "just for a simple case" reintroduces exactly the kind of boundary erosion `14_workflow_engine.md` Section 7 warns against in a different context — the principle is the same regardless of which module is tempted to take the shortcut.
3. **Freshness transparency carries through:** Since Analytics read models carry a `projectionFreshness`/`updated_at` marker (`08_database_design.md` Section 11.1), Engagement can (and per `17_observability.md`, should) log/monitor how fresh the data underlying a given notification was at composition time — a diagnostic capability that would not exist if Engagement computed its own numbers ad hoc.

---

## 11. Component Responsibilities

| Component | Responsibility |
|---|---|
| Preference Gate | Enforce per-notification-type opt-out before any composition work begins |
| Content Composer | Read Analytics models, select template, map values to placeholders |
| Template Renderer | Final placeholder substitution against Markdown or Meta HSM shape |
| Messaging Client (Port) | Unified interface (`SendMessage`) abstracting external delivery details |
| Provider Adapters (`fonnte`, `meta`, `mock`) | Specific API communication, rate-limit-aware sending, provider-specific retry classification |
| Reply Webhook Handler | Structured button-reply and whitelisted-text-reply interpretation for both Fonnte and Meta |
| Delivery Status Webhook Handler | Two-tier (delivered/read) confirmation tracking mapped from provider-specific callbacks |

---

## 12. Trade-offs

| Decision | Trade-off |
|---|---|
| Small, curated pre-approved template catalog (vs. dynamic LLM-composed messages) | Less content flexibility/personalization in message phrasing than a fully dynamic approach, in exchange for actually being usable at all under WhatsApp's business-messaging policy — this is not optional given the platform constraint (Section 4.1), so framed correctly it is a constraint satisfied, not merely a trade-off chosen. |
| Interactive-buttons-first, text-pattern-fallback reply handling | More implementation surface (two reply-handling code paths) than a single free-text NLU approach, in exchange for near-zero ambiguity risk in a flow that directly writes to the financial ledger. |
| Read-receipt tracking as observability-only, never a functional gate | Slightly less potentially-available signal used for engagement logic, in exchange for robustness against the real-world unreliability of read-receipt availability. |

## 13. Advantages
- The template catalog design directly and fully resolves H7 with a concrete, submittable artifact (Section 4.2's table) rather than leaving the pre-approval constraint as an abstract risk.
- The structured-reply-first design for H4 means the platform's most safety-critical WhatsApp interaction (confirming/correcting a financial event) carries essentially the same reliability guarantees as a native app UI button tap, despite happening over a messaging channel.
- Sourcing composition exclusively from Analytics read models guarantees numeric consistency between every user-facing surface (WhatsApp summaries, future in-app dashboards) by architectural construction, not by convention.

## 14. Disadvantages
- The template-approval lead time (Section 4.3) is an external dependency the engineering team does not fully control, meaning any new notification type requires forward planning that a purely in-house feature would not — this must be explicitly factored into sprint/release planning for any Engagement-touching feature.
- The quality-rating governance requirement (Section 9) means notification *frequency and relevance* decisions are not purely a product/UX concern but carry platform-availability stakes — this creates a cross-functional dependency (product decisions affecting engineering risk) that must be actively managed, not merely documented once.

## 15. Future Improvements
- Once Phase 3's conversational AI assistant is prioritized (`01_product_vision.md` Stage 4), revisit whether a constrained-but-richer reply-parsing model (still structured, but supporting a wider intent set than Confirm/Edit) is warranted — any expansion must still pass through the same "structured-first, whitelisted-fallback, never open NLU for ledger-affecting replies" discipline established here.
- Evaluate WhatsApp Business API's higher-tier "Flows" feature (structured multi-step in-chat forms) as a potential upgrade to the current button/text `[Edit]` correction flow (Section 6.1), if it offers a materially better structured-correction UX without sacrificing the no-open-NLU guarantee.
- Build automated template-approval-status monitoring (polling or webhook-driven) so a template's approval/rejection is reflected in the platform's own configuration state without manual follow-up, reducing the operational overhead of Section 4.3's process.

## 16. Best Practices
- No notification content may be composed from anything other than an approved template ID present in the platform's template catalog configuration — attempting to send with an unmapped `notification_type` must fail loudly (a configuration error) rather than attempt free-form composition.
- Every reply-webhook handler change must be tested against WhatsApp's documented interactive-message payload shapes in `18_testing_strategy.md`'s fixture set, including the degraded-fallback text scenarios (Section 6.2), not only the happy-path button interaction.
- Quality-rating-relevant metrics (block rate, opt-out rate — Section 9) must be reviewed on a standing cadence (e.g., monthly) by both engineering and product, not only monitored reactively after a rating degradation is already underway.

## 17. Production Considerations
- The messaging-tier limit (Section 9) must be tracked as a live, monitored ceiling against actual daily send volume, with proactive alerting well before the ceiling is approached — hitting the limit mid-day would cause silent, deferred, or dropped notifications for affected users, a direct product-quality incident.
- Template approval status must be verified as part of any deployment that introduces a new/changed `notification_type` — a deploy that ships code assuming an unapproved template exists would fail at the delivery step in production, not at build time, making this a mandatory pre-deployment checklist item (`19_deployment.md`).
- The `[Edit]` correction flow's session-window dependency (Section 6.1 — follow-up messages are only permitted within 24 hours of the user's last message) must be handled gracefully if a user takes longer than 24 hours to complete a multi-step correction — the platform must detect this and re-initiate via a new approved template rather than silently failing to deliver the follow-up.

---

**Next Document:** `16_security.md` — specifies the platform's full security architecture: OAuth token encryption and key management, RBAC for the `/admin/*` surface, PII-minimization and data-handling controls (referenced from `11_ai_blueprint.md` and this document), GDPR compliance mechanics, and the audit-logging design underpinning every module's compliance obligations.
