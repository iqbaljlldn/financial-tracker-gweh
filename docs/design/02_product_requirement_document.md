# Product Requirement Document (PRD)
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`
**Feeds Into:** `03_software_requirement_specification.md`, `04_domain_model.md`, `09_api_specification.md`

---

## 1. Purpose

This document translates the strategic vision established in `01_product_vision.md` into **concrete, testable, and prioritized product requirements**. Where the Vision document answers "why does this exist," this PRD answers "what must the system do, for whom, and how do we know it's done."

This is the contract between Product and Engineering. Every feature listed here must be traceable forward into the SRS (`03_software_requirement_specification.md`), the domain model (`04_domain_model.md`), and ultimately into API contracts (`09_api_specification.md`). Nothing in the backend blueprint or database design should introduce a capability that isn't rooted in a requirement stated here, and nothing stated here should be left unimplemented without an explicit, documented decision.

## 2. Scope

### 2.1 In Scope
- Functional requirements for all core features listed in the project brief: Authentication, User Management, Gmail Connector, Email Processing, AI Engine, Ledger, Analytics, Budget, Notification, Administration.
- User stories with acceptance criteria for each functional area.
- Prioritization (MoSCoW) for MVP vs. later phases.
- Non-functional requirements **at the product level** (detailed engineering-grade NFRs live in the SRS).
- Success metrics and KPIs.

### 2.2 Out of Scope
- Technical architecture and technology choices → `06_architecture.md`
- Database schema → `08_database_design.md`
- API request/response contracts → `09_api_specification.md`
- Detailed AI prompt design → `11_ai_blueprint.md`
- Detailed security controls → `16_security.md`

---

## 3. Architecture (Product-Level Feature Map)

```
Personal Finance Assistant Platform
│
├── Identity & Access
│   ├── Google OAuth Authentication
│   └── Multi-Account Gmail Management
│
├── Ingestion Layer
│   ├── Gmail Connector (Watch, Push, Full Sync, Incremental Sync)
│   └── [Future Connectors — see 24_future_roadmap.md]
│
├── Understanding Layer
│   ├── Email Processing (Sender ID, Template/Regex Parsing, Duplicate Detection)
│   └── AI Extraction Engine (Merchant, Amount, Currency, Date, Category, Payment Method, Confidence)
│
├── Financial Core
│   ├── FinancialEvent Ledger (Income, Expense, Transfer, Refund, Cashback, Subscription
│   │   Renewal, Failed Payment, Salary, Investment, Dividend, Loan, Credit Card Bill,
│   │   Tax, Donation, Others)
│   ├── Analytics Engine (Cash Flow, Category Breakdown, Merchant Ranking, Trends)
│   └── Budget Engine (Monthly Budget, Overspending Alerts, Savings Goals)
│
├── Engagement Layer
│   └── WhatsApp Notification (Daily/Weekly/Monthly Summary, AI Recommendations)
│
└── Administration
    ├── Connector Monitoring
    ├── Parser Monitoring
    ├── Prompt Monitoring
    ├── Audit Log
    └── User Settings
```

---

## 4. Functional Requirements

Each requirement is tagged with a unique ID (`FR-<Module>-<Number>`), a MoSCoW priority, and acceptance criteria. IDs are stable identifiers referenced by later documents (test plans, API spec, sequence diagrams).

### 4.1 Authentication & Identity

**FR-AUTH-01 — Google OAuth Sign-In** *(Must Have)*
- The user authenticates exclusively via Google OAuth 2.0 (Authorization Code flow with PKCE).
- No username/password authentication is offered for the primary flow.
- **Acceptance Criteria:**
  - Given a new user, when they initiate sign-in, then they are redirected to Google's consent screen requesting only the minimum required scopes (`gmail.readonly`, `userinfo.email`, `userinfo.profile`).
  - Given a successful consent, then a platform session (JWT/opaque session token) is issued and the user record is created or matched.
  - Given the user denies consent, then the system displays a clear explanation and does not create a partial account.

**FR-AUTH-02 — Token Refresh & Revocation Handling** *(Must Have)*
- The system transparently refreshes OAuth access tokens using stored refresh tokens.
- The system detects and reacts to token revocation (user removed app access from Google Account settings) by marking the connector as disconnected and notifying the user.
- **Acceptance Criteria:**
  - Given an expired access token, when a Gmail API call is made, then the system refreshes it transparently without user interruption.
  - Given a revoked refresh token, when the next scheduled sync runs, then the connector status transitions to `disconnected` and the user receives a reconnection prompt via WhatsApp/email.

### 4.2 User Management

**FR-USER-01 — Multiple Gmail Account Linking** *(Should Have)*
- A single platform user may link more than one Gmail account (e.g., personal + business).
- Each linked Gmail account operates as an independent ingestion source, but all events roll up into a single user ledger.
- **Acceptance Criteria:**
  - Given a user with one linked Gmail account, when they initiate "Add Gmail Account," then a second OAuth consent flow is triggered and, upon success, a second connector instance is created under the same user.
  - Given two linked accounts, when analytics are computed, then events from both accounts are merged into a single cash flow view, tagged with their source account for traceability.

**FR-USER-02 — Account Disconnection & Data Retention Choice** *(Must Have)*
- Users can disconnect a Gmail account at any time.
- On disconnection, the user chooses to (a) retain historical financial events already extracted, or (b) purge them.
- **Acceptance Criteria:**
  - Given a disconnect request, then Gmail watch/push subscriptions for that account are terminated within one sync cycle.
  - Given the user selects "purge historical data," then all `FinancialEvent` records sourced from that connector are soft-deleted and scheduled for permanent erasure per the GDPR retention policy (`16_security.md`).

### 4.3 Gmail Connector

**FR-GMAIL-01 — Initial Full Sync** *(Must Have)*
- On first connection, the system performs a bounded historical scan (configurable window, default last 90 days) to seed the ledger.
- **Acceptance Criteria:**
  - Given a newly connected Gmail account, when the initial sync runs, then all matching transaction-like emails within the configured window are enqueued for processing.
  - Given the initial sync is in progress, then the user sees a "syncing" status and an estimated completion indicator.

**FR-GMAIL-02 — Real-Time Watch via Push Notifications** *(Must Have)*
- The system registers a Gmail Watch on the user's mailbox and receives push notifications (via Pub/Sub) for new mail.
- **Acceptance Criteria:**
  - Given an active watch, when a new transaction email arrives, then it is processed within an SLA of ≤ 2 minutes from arrival (P95).
  - Given a Gmail Watch expiration (7-day max lifetime per Gmail API), then the system renews the watch automatically before expiry without data loss.

**FR-GMAIL-03 — History-Based Incremental Sync** *(Must Have)*
- The system uses Gmail's History API (`historyId`) to fetch only the delta since the last successfully processed point, rather than re-scanning the mailbox.
- **Acceptance Criteria:**
  - Given a resumed sync after downtime, then only messages added/changed since the last stored `historyId` are fetched.
  - Given a `historyId` that has expired/is invalid (Gmail retention limits), then the system falls back to a bounded full re-sync and logs the fallback event.

**FR-GMAIL-04 — Sender Identification** *(Must Have)*
- Every ingested email is matched against a known-sender registry (bank/e-wallet/merchant sender domains and patterns).
- **Acceptance Criteria:**
  - Given an email from a registered sender, then it is routed to the Template Parser.
  - Given an email from an unregistered sender that is still classified as transaction-like, then it is routed to the AI Extractor.
  - Given an email that is not transaction-like at all (e.g., a newsletter), then it is discarded before extraction with an auditable reason code.

### 4.4 Email Processing

**FR-PARSE-01 — Template/Regex Parsing** *(Must Have)*
- Known senders have versioned templates/regex definitions that deterministically extract merchant, amount, currency, date, and payment method.
- **Acceptance Criteria:**
  - Given a known template match, then extraction completes without any AI/LLM call.
  - Given a template parse failure (e.g., sender changed their email format), then the system automatically falls back to the AI Extractor and flags the template for review in Parser Monitoring.

**FR-PARSE-02 — Duplicate Detection** *(Must Have)*
- The system must never create two `FinancialEvent` records for the same underlying email or the same underlying financial occurrence.
- **Acceptance Criteria:**
  - Given the same Gmail message ID delivered twice (push + sync overlap), then only one `FinancialEvent` is persisted.
  - Given two emails describing the same transaction from different notification channels (e.g., bank SMS-to-email gateway and direct bank email, future scope), then the system supports a secondary fuzzy-match dedup strategy (amount + merchant + timestamp window) — flagged as a `Should Have` for the connector-expansion phase.

**FR-PARSE-03 — Retry on Transient Failure** *(Must Have)*
- Any transient failure (API timeout, rate limit, downstream AI provider error) triggers a retry with backoff, not a silent drop.
- **Acceptance Criteria:**
  - Given a transient failure during extraction, then the job is retried up to a configured max attempt count with exponential backoff.
  - Given max retries exhausted, then the message is routed to a dead-letter queue and surfaced in Administration → Parser Monitoring.

### 4.5 AI Engine

**FR-AI-01 — Structured Extraction from Unknown Senders** *(Must Have)*
- For emails not matched to a known template, the AI Extractor produces a structured candidate `FinancialEvent` (merchant, amount, currency, date, category, payment method) plus a confidence score.
- **Acceptance Criteria:**
  - Given an unknown-sender transaction email, then the AI Extractor returns a structured JSON candidate conforming to the platform's extraction schema.
  - Given the AI cannot confidently extract an amount or merchant, then the confidence score reflects this (low) and the event is routed to the Validation/Review queue rather than being auto-accepted.

**FR-AI-02 — AI Categorization** *(Must Have)*
- Every `FinancialEvent` (regardless of extraction path) is assigned a category by the AI Categorization engine, informed by merchant name, historical user categorization, and amount patterns.
- **Acceptance Criteria:**
  - Given a new event with a known merchant (previously categorized by the same user), then the system applies the learned category with high confidence.
  - Given a user manually corrects a category, then the correction is stored and influences future categorization for that merchant/user pair.

**FR-AI-03 — Confidence Scoring** *(Must Have)*
- Every AI-derived field carries an explicit confidence score (0.0–1.0), and every event carries an overall confidence score.
- **Acceptance Criteria:**
  - Given an overall confidence below the configured threshold (default 0.7), then the event is not auto-finalized and is queued for user confirmation via notification.
  - Given an overall confidence at/above threshold, then the event is auto-finalized into the ledger.

**FR-AI-04 — Prompt Versioning** *(Must Have)*
- Every AI extraction/categorization call is tagged with the prompt template version used.
- **Acceptance Criteria:**
  - Given any AI-derived `FinancialEvent`, then it is possible to trace back the exact prompt version and model used, for audit and regression-debugging purposes.
  - Given a prompt version is rolled back or updated, then historical events remain attributed to the version active at the time of their creation (immutable attribution).

### 4.6 Ledger

**FR-LEDGER-01 — FinancialEvent Recording** *(Must Have)*
- All validated events are persisted as `FinancialEvent` records with a type from the extensible taxonomy (Income, Expense, Transfer, Refund, Cashback, Subscription Renewal, Failed Payment, Salary, Investment, Dividend, Loan, Credit Card Bill, Tax, Donation, Others).
- **Acceptance Criteria:**
  - Given a validated extraction, then a `FinancialEvent` is persisted with full source traceability (connector, message reference, parser/prompt version, confidence).
  - Given an event type not yet explicitly modeled, then it is captured under `Others` without extraction failure, and flagged for taxonomy review.

**FR-LEDGER-02 — Manual Correction & Override** *(Should Have)*
- Users may manually edit an event's category, amount, or type (for cases where automated extraction is wrong), without needing to disable automation entirely.
- **Acceptance Criteria:**
  - Given a user edits a field, then the change is recorded as a manual override with full audit trail (previous value, new value, timestamp, actor).
  - Given a manual override exists, then future automated re-processing (e.g., re-parse on template update) never silently overwrites the user's manual correction.

**FR-LEDGER-03 — Manual Event Entry (Fallback)** *(Could Have)*
- As a fallback for sources not yet connected, users may manually add a `FinancialEvent`.
- **Acceptance Criteria:**
  - Given a manual entry, then it is stored with `source = manual` and is fully compatible with analytics/budget/notification consumers.

### 4.7 Analytics

**FR-ANALYTICS-01 — Cash Flow View** *(Must Have)*
- Aggregated inflow vs. outflow over a selected period.

**FR-ANALYTICS-02 — Category Breakdown** *(Must Have)*
- Spend/income aggregated by category over a selected period, with percentage share.

**FR-ANALYTICS-03 — Merchant Ranking** *(Should Have)*
- Top merchants by spend volume and frequency over a selected period.

**FR-ANALYTICS-04 — Monthly & Daily Trend** *(Must Have)*
- Time-series views of income/expense at daily and monthly granularity, supporting month-over-month comparison.

*(Acceptance criteria for all analytics requirements follow a common pattern: Given a date range and user, when the analytics endpoint is queried, then results reflect only finalized — non-pending-review — `FinancialEvent` records for that user, computed correctly against known test fixtures. Detailed test cases are enumerated in `18_testing_strategy.md`.)*

### 4.8 Budget Engine

**FR-BUDGET-01 — Monthly Budget Definition** *(Must Have)*
- Users define a monthly budget, globally or per category.

**FR-BUDGET-02 — Overspending Alert** *(Must Have)*
- The system detects when actual spend approaches or exceeds a defined budget threshold (e.g., 80%, 100%) and triggers a notification.
- **Acceptance Criteria:**
  - Given a category budget of X and cumulative spend reaching 80% of X within the period, then a WhatsApp alert is sent once (not repeatedly) per threshold crossing.

**FR-BUDGET-03 — Savings Goal Tracking** *(Should Have)*
- Users define a savings goal (target amount, target date); the system tracks progress based on net cash flow.

### 4.9 Notification (WhatsApp)

**FR-NOTIF-01 — Daily Summary** *(Must Have)*
- A daily digest of the prior day's financial events, sent at a user-configurable time.

**FR-NOTIF-02 — Weekly Summary** *(Must Have)*
- A weekly digest including cash flow, top categories, and notable events (large expenses, failed payments).

**FR-NOTIF-03 — Monthly Report** *(Must Have)*
- A monthly digest including budget performance, trend comparison to the previous month, and savings goal progress.

**FR-NOTIF-04 — AI Recommendation Messages** *(Could Have)*
- Periodic proactive recommendations (e.g., unused subscriptions, spend anomalies) delivered via WhatsApp.

**FR-NOTIF-05 — Delivery Reliability & Opt-Out** *(Must Have)*
- Notification delivery is retried on transient WhatsApp API failure; users can opt out of any notification category independently.
- **Acceptance Criteria:**
  - Given a WhatsApp delivery failure, then the system retries per the platform's standard retry policy before marking as failed and logging for Administration visibility.
  - Given a user opts out of "Daily Summary" specifically, then Weekly/Monthly notifications remain unaffected.

### 4.10 Administration

**FR-ADMIN-01 — Connector Monitoring** *(Must Have)*
- Visibility into each connector's health: last sync time, sync lag, error rate, watch expiration status.

**FR-ADMIN-02 — Parser Monitoring** *(Must Have)*
- Visibility into template parser success/failure rates per sender, with alerting when a previously reliable template's failure rate spikes (indicating a sender template change).

**FR-ADMIN-03 — Prompt Monitoring** *(Should Have)*
- Visibility into AI extraction/categorization confidence distributions per prompt version, enabling detection of prompt regression.

**FR-ADMIN-04 — Audit Log** *(Must Have)*
- An immutable, queryable log of all significant state-changing actions: OAuth connect/disconnect, manual overrides, budget changes, data purges.

**FR-ADMIN-05 — User Settings** *(Must Have)*
- User-manageable preferences: notification timing/opt-outs, budget thresholds, category taxonomy customization, data retention choices.

---

## 5. Non-Functional Requirements (Product-Level)

*(Full engineering-grade NFRs with measurable targets are specified in `03_software_requirement_specification.md`. This section states them at product intent level.)*

| NFR | Product-Level Statement |
|---|---|
| Availability | The ingestion pipeline and notification delivery must be highly available; a brief API outage must never cause data loss (durable queuing). |
| Scalability | The system must support growth from hundreds to millions of linked mailboxes without architectural rewrite. |
| Security | Financial and email data are highly sensitive; encryption, least-privilege OAuth scopes, and strict access control are baseline. |
| Data Integrity | No duplicate financial events; every event traceable to its source and processing lineage. |
| Latency | New transaction emails should reflect in the ledger within minutes, not hours. |
| Compliance | GDPR-friendly data handling: export, deletion, and processing transparency. |
| Extensibility | Adding a new connector or FinancialEvent type must not require breaking changes to existing modules. |

---

## 6. Detailed Explanation: MVP vs. Later Phases

### 6.1 MVP (Phase 1) — "Observation + Understanding"
- Google OAuth authentication (single Gmail account per user).
- Gmail Connector: initial full sync, push-based watch, incremental sync.
- Template/regex parsing for a curated set of high-volume known senders (top banks/e-wallets in the initial target market).
- AI Extractor as fallback for unknown senders.
- Confidence scoring + validation gate.
- FinancialEvent Ledger covering Income, Expense, Transfer, Refund at minimum; other types captured under `Others` if not yet fully modeled.
- Basic Analytics: Cash Flow, Category Breakdown, Monthly/Daily Trend.
- WhatsApp Daily/Weekly/Monthly Summary.
- Administration: Connector Monitoring, Parser Monitoring, Audit Log (minimum viable).

### 6.2 Phase 2 — "Multi-Account + Budget + Deeper AI"
- Multiple Gmail account linking.
- Budget Engine (monthly budgets, overspending alerts).
- Merchant Ranking analytics.
- Prompt Monitoring in Administration.
- Manual correction/override workflows.

### 6.3 Phase 3 — "Advisory Intelligence"
- Savings Goal tracking.
- AI Recommendation messages.
- Anomaly detection (duplicate charge, fraud-pattern signals, failed payment cascades).
- Conversational query interface via WhatsApp.

*(This phasing directly informs `24_future_roadmap.md` and is the basis for release planning; it is not a rigid engineering sequencing constraint — architecture must support all phases from Phase 1, per the Vision document's guiding principles.)*

---

## 7. Component Responsibilities

| Component | Product Responsibility |
|---|---|
| Gmail Connector | Reliable, timely, duplicate-free ingestion of raw email signals |
| Email Processing | Correctly route known vs. unknown senders; guarantee no duplicate processing |
| AI Engine | Produce structured, confidence-scored candidates where deterministic parsing cannot |
| Ledger | Be the single, trustworthy source of truth for all financial events |
| Analytics | Provide accurate, correct-by-construction aggregate views derived only from finalized events |
| Budget Engine | Track user-defined thresholds and trigger timely alerts |
| Notification | Deliver timely, opt-in-respecting, reliable WhatsApp communication |
| Administration | Give the operations team visibility to catch parser/prompt/connector degradation before users notice |

---

## 8. Trade-offs

| Decision | Trade-off |
|---|---|
| Curated known-sender list at MVP rather than "AI for everything" | Requires upfront template-building effort per sender, in exchange for lower cost and higher reliability for the highest-volume senders from day one. |
| Confidence threshold gating (auto-finalize vs. review queue) | Introduces a "pending review" user experience state, in exchange for preventing incorrect data from silently entering a financial ledger. |
| Single Gmail account at MVP, multi-account in Phase 2 | Slightly narrower initial addressable use case, in exchange for faster MVP delivery and simpler initial data model validation. |
| `Others` catch-all FinancialEvent type at MVP | Some events won't be perfectly typed initially, in exchange for never blocking ingestion on taxonomy completeness. |

## 9. Advantages
- Requirements are directly traceable to the Vision's Stage 1–4 trajectory — nothing here contradicts or narrows that trajectory.
- MoSCoW prioritization gives Engineering an unambiguous MVP boundary without under-designing for later phases.
- Acceptance criteria are testable and directly usable by QA (`18_testing_strategy.md`) without reinterpretation.

## 10. Disadvantages
- The breadth of Phase 1 (multiple parser paths, AI fallback, confidence gating, WhatsApp integration) means MVP is not "minimal" in engineering effort, even though it's minimal in user-facing surface area — this is a deliberate, vision-driven trade-off, not an oversight.
- Reliance on a curated known-sender list means initial market coverage is bounded by how many bank/e-wallet templates are built pre-launch; this is a content/ops dependency, not purely an engineering one.

## 11. Future Improvements
- Expand known-sender template library continuously post-launch, informed by Parser Monitoring data (which unknown senders generate the highest AI-extraction volume).
- Introduce user-facing confidence transparency ("this transaction was auto-detected with 92% confidence") to build trust progressively.
- Extend the review-queue UX into a conversational WhatsApp confirmation flow ("Was this $45 charge from 'AMZN Mktp' a shopping expense? Reply Y/N") to keep the zero-friction promise even during low-confidence cases.

## 12. Best Practices
- Every functional requirement must map to at least one entity/relationship in `04_domain_model.md` — no orphan requirements.
- Every "Must Have" MVP requirement must have a corresponding acceptance test defined before implementation begins.
- Confidence thresholds must be configurable (not hardcoded) to allow tuning post-launch based on real extraction accuracy data.

## 13. Production Considerations
- Known-sender templates must be versioned and deployable independently of core application releases, to allow rapid response to sender format changes without a full deployment cycle.
- The review-queue (low-confidence events) must have a bounded SLA for user action; stale pending events should not silently accumulate forever — a reconciliation/expiry policy is required (detailed in `08_database_design.md` and `02_product_requirement_document.md`'s Phase 2 budget/analytics exclusion rules).
- WhatsApp Business API has messaging template approval and rate constraints; notification requirements (FR-NOTIF-*) must be implementable within those constraints — cross-checked in `15_notification_system.md`.

---

## 14. Success Metrics (Product-Level KPIs)

| Metric | Target Intent |
|---|---|
| Time-to-first-insight | User receives their first WhatsApp summary within 24 hours of connecting Gmail |
| Extraction accuracy | ≥ 95% of finalized (auto-accepted) events are correct without manual correction |
| Auto-finalization rate | ≥ 85% of transaction emails from known senders are auto-finalized without review queue |
| Notification delivery success rate | ≥ 99% successful WhatsApp delivery (excluding user opt-out) |
| Connector uptime | ≥ 99.9% Gmail watch/sync availability |
| Time-to-detect sender template break | Parser failure spike detected and alerted within 1 hour |

---

## 15. Traceability Note

Every `FR-*` ID in this document is expected to be referenced by:
- `04_domain_model.md` (entity/behavior mapping)
- `09_api_specification.md` (endpoint mapping)
- `18_testing_strategy.md` (test case mapping)

This traceability chain is mandatory for engineering review sign-off and must not be broken by later documents introducing unmapped capabilities.

---

**Next Document:** `03_software_requirement_specification.md` — formalizes these product requirements into engineering-grade functional and non-functional specifications, including measurable NFR targets, constraints, and system boundaries.
