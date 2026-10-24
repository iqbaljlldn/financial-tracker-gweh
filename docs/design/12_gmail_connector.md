# Gmail Connector
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `03_software_requirement_specification.md`, `06_architecture.md`, `07_sequence_diagrams.md`, `08_database_design.md`, `10_backend_blueprint.md`
**Feeds Into:** `13_email_parser.md`, `16_security.md`, `24_future_roadmap.md`

---

## 1. Purpose

This document specifies the **Gmail Connector** — the reference implementation of the platform's Connector abstraction (`01_product_vision.md` Section 8, Principle 1) — in full implementation-ready detail. It covers OAuth scope handling, the Gmail Watch/History API integration mechanics, sender-classification heuristics, and the concrete engineering resolution of the dual-trigger ingestion design (H2, from `05_event_storming.md`/`06_architecture.md` Section 6.6) at the connector level.

Because Gmail is deliberately chosen as the hardest connector to build first (per the Vision's rationale), this document also explicitly identifies **which parts of this design are Gmail-specific vs. which parts define the general shape every future connector (Outlook, IMAP, SMS, etc.) must also implement** — this boundary is what makes the Connector abstraction real rather than aspirational.

## 2. Scope

### 2.1 In Scope
- The `Connector` interface contract (general shape, Gmail as reference implementation).
- Gmail OAuth scope and consent handling.
- Gmail Watch registration/renewal mechanics.
- History API incremental sync mechanics, including `historyId` invalidation handling.
- Initial full sync mechanics and bounding strategy.
- Sender classification heuristics (routing to template parser vs. AI extractor vs. discard).
- Gmail API quota management.
- Failure modes specific to the Gmail API surface.

### 2.2 Out of Scope
- General ingestion architecture (queues, Outbox/Inbox) → `06_architecture.md` Section 6.
- Email parsing/extraction logic itself → `13_email_parser.md`, `11_ai_blueprint.md`.
- OAuth token encryption/storage security controls → `16_security.md`.

---

## 3. Architecture: The Connector Abstraction

### 3.1 General Connector Interface (Conceptual Contract)

Every current and future transaction source implements the same conceptual contract, ensuring the Ingestion module's application layer (`10_backend_blueprint.md`) never special-cases Gmail at the interface level:

| Interface Method (conceptual) | Purpose |
|---|---|
| `Authenticate(ctx, credentials) (ConnectorAccount, error)` | Establish and validate a source-specific account link |
| `InitialSync(ctx, account, window) (SyncHandle, error)` | Perform a bounded historical capture |
| `RegisterRealtimeWatch(ctx, account) (WatchHandle, error)` | Register for real-time/near-real-time new-signal notification, if the source supports it |
| `FetchIncremental(ctx, account, cursor) ([]RawMessage, newCursor, error)` | Fetch only new signals since the last cursor |
| `Disconnect(ctx, account) error` | Cleanly terminate the connection and any registered watches |

**Gmail-specific concepts (message IDs, `historyId`, Pub/Sub watch semantics, MIME structure) are confined entirely to the Gmail implementation of this interface** — no Gmail-specific type ever crosses into the `understanding` module or beyond, satisfying the Vision's Guiding Principle 1 (`01_product_vision.md` Section 8) precisely.

### 3.2 Gmail as Reference Implementation

```
┌────────────────────────────────────────────────────────────┐
│  ingestion module (general, connector-agnostic application    │
│  layer — command/query/policy handlers per 10_backend_blueprint)│
└───────────────────────┬────────────────────────────────────┘
                          │  implements/consumes the Connector interface
                          ▼
┌────────────────────────────────────────────────────────────┐
│  ingestion/adapter/external/gmailclient.go                    │
│  (Gmail-specific: OAuth, Watch, History API, MIME parsing entry)│
└───────────────────────┬────────────────────────────────────┘
                          │
                          ▼
                    Gmail API (Google)
```

---

## 4. OAuth Scope & Consent Handling

### 4.1 Requested Scopes

Per SRS-FR-AUTH-01 and the Vision's least-privilege principle:

| Scope | Purpose | Justification |
|---|---|---|
| `https://www.googleapis.com/auth/gmail.readonly` | Read-only mailbox access | The **only** Gmail-specific scope requested — the platform never sends, modifies, labels, or deletes mail. This is a hard, non-negotiable constraint referenced explicitly in `16_security.md`. |
| `openid`, `email`, `profile` | User identity | Standard identity scopes for account creation/matching (Identity & Access context). |

**Explicitly never requested:** `gmail.modify`, `gmail.compose`, `gmail.send`, `gmail.settings.*` — any of these would violate the least-privilege principle and materially increase the platform's risk surface and users' reasonable trust expectations for a read-only observational product.

### 4.2 Consent Flow Parameters

- `access_type=offline` — mandatory to receive a refresh token (without it, Google issues only a short-lived access token, insufficient for continuous background sync).
- `prompt=consent` — forces the consent screen even for a returning user, guaranteeing a refresh token is (re-)issued on every linking flow (Google does not reliably re-issue a refresh token on repeat authorization without this parameter, a common integration pitfall this document explicitly calls out to prevent).
- `include_granted_scopes=true` — supports the future `FR-USER-01` multi-account scenario cleanly, allowing incremental authorization if scope needs ever expand (though no scope expansion is currently planned).

### 4.3 Consent Revocation Detection

Per SRS-FR-AUTH-02: token refresh failures are classified server-side:
- `invalid_grant` response from Google's token endpoint → **terminal**: the user has revoked access (either via Google Account settings or by uninstalling/revoking third-party access). The connector transitions `gmail_accounts.status` to `disconnected` (not `error`, since this is an expected, user-initiated-elsewhere state, not a bug) and triggers a reconnection-prompt notification per `05_event_storming.md`'s `AccountDisconnected` policy chain.
- Network/5xx errors from the token endpoint → **transient**: standard retry/backoff per `06_architecture.md` Section 6.4, no status change.

---

## 5. Gmail Watch Mechanics

### 5.1 Registration

- Uses the Gmail API's `users.watch` method, registering the mailbox against a **platform-owned Google Cloud Pub/Sub topic**.
- The topic is configured with a push subscription targeting the platform's `/webhooks/gmail/pubsub` endpoint (`09_api_specification.md` Section 6.7).
- **Per-user topic isolation is NOT used** (a single shared topic serves all users, with the account identity resolved server-side from the push payload's `emailAddress` field) — using one Pub/Sub topic per user would not scale operationally at the platform's target of 100,000+ mailboxes (`03_software_requirement_specification.md` Section 6.5) and provides no meaningful additional security benefit given the webhook's own signature verification (Section 4.4 of `09_api_specification.md`).

### 5.2 Renewal

- Gmail Watch has a **hard 7-day maximum lifetime** (a Google API constraint, not a platform design choice) — `03_software_requirement_specification.md` Section 7 lists this as a binding external constraint.
- The connector schedules renewal at **≤ 6 days** from registration (1-day safety margin), via a periodic Asynq task (`ingestion:critical` queue, per `06_architecture.md` Section 6.3) that re-issues `users.watch` for every `gmail_account` whose `watch_expiration` falls within the next 24 hours.
- Renewal is itself idempotent — re-calling `users.watch` before expiry simply extends the watch and returns a new `expiration` and (occasionally) a new `historyId` baseline, both persisted to `gmail_accounts` (per `08_database_design.md` Section 8.1).

### 5.3 The Dual-Trigger Design (H2 Resolution, Connector-Level Detail)

As established at the architecture level (`06_architecture.md` Section 6.6), the connector implements **two independent triggers** for `FetchIncremental`:

1. **Push-triggered:** Pub/Sub webhook → verified → `RunIncrementalSync` enqueued immediately.
2. **Poll-triggered (safety net):** A per-account scheduled task, independent of push health, at a fixed interval — **default 15 minutes**, configurable via Viper.

**Why 15 minutes specifically (not shorter or longer):** This value directly trades off Gmail API quota consumption (Section 8) against the maximum possible "silent gap" duration if push delivery is failing. At 15-minute polling, a worst-case complete push failure still bounds the finalization-latency degradation to roughly 15 minutes beyond the SRS's P95 targets (Section 6.1 of `03_software_requirement_specification.md`) — an acceptable degraded-mode bound, not a violation of the primary (push-healthy) SLA, which remains ≤2/5 minutes. This value is explicitly flagged as **production-tunable** based on observed push reliability and quota headroom (Section 8), not a fixed constant.

---

## 6. History API Incremental Sync

### 6.1 Normal Operation

- `FetchIncremental` calls Gmail's `history.list` with the stored `last_history_id`, filtered to `historyTypes=messageAdded`.
- Each returned message triggers the `MessageIngested` flow (`07_sequence_diagrams.md` Diagram 2).
- `last_history_id` is updated to the response's new `historyId` **only after all messages in the batch are durably recorded** (transactional, per `08_database_design.md` Section 8.2's ingestion table design) — ensuring a mid-batch crash results in safe reprocessing of the same batch on the next trigger (idempotent via the `(gmail_account_id, gmail_message_id)` unique constraint), never a skipped message.

### 6.2 `historyId` Expiration Handling

Google retains history records for a limited window (typically around 7-30 days, not contractually guaranteed and subject to change — treated as an operational constraint, not a fixed number this document hardcodes). When `history.list` returns a `404`:

1. The connector does **not** fall back to a full 90-day re-sync (excessive, wasteful, and unnecessary given the bounded gap).
2. Instead, it performs a **bounded fallback re-sync**: `messages.list` with a query filter bounding to the last N days (default 3, configurable), cross-referenced against `ingested_messages` to skip already-processed messages (via the same unique-constraint idempotency).
3. This event is explicitly logged as `HistorySyncFallbackTriggered` (per `04_domain_model.md` Section 6.4's domain event) and surfaced in Connector Monitoring (FR-ADMIN-01) — an operational signal that push+poll reliability may need review if this triggers unexpectedly often for a given account (it should be rare under normal dual-trigger operation, since the poll fallback in Section 5.3 should keep `historyId` reasonably fresh).

### 6.3 Initial Full Sync

Per SRS-FR-GMAIL-01:
- Bounded to a configurable lookback window (default 90 days).
- Uses `messages.list` with a **query filter** narrowing candidates before they're even enqueued for classification — reducing wasted downstream processing on obviously non-financial mail. The query filter combines: (a) a curated list of known financial-sender domain patterns (OR-combined), and (b) a keyword heuristic fallback (subject/snippet containing transaction-suggestive terms) for senders not yet in the known-sender registry — this second branch is what allows the initial sync to still surface unknown-sender transactional emails for AI extraction, rather than only ever finding known senders.
- Paginated via Gmail's `pageToken` mechanism; each page's messages are enqueued as `MessageIngested` within their own transaction, and the **overall sync progress (last successful page token)** is persisted, so a resumed sync after downtime continues from the correct page rather than restarting or skipping content (per SRS-FR-GMAIL-01's error-handling clause).
- On completion, the final `historyId` observed during the sync is stored as the connector's `last_history_id` baseline, transitioning the account from `initial_sync` to `active` status.

---

## 7. Sender Classification Heuristics

Per SRS-FR-GMAIL-04, every ingested message is classified before extraction:

### 7.1 Classification Pipeline

1. **Known-sender exact/pattern match:** Sender address/domain checked against the `understanding` module's `sender_templates` active registry (via the cross-module interface pattern — `ingestion` calls a narrow, published `understanding` query interface, or more precisely, per `04_domain_model.md` Section 13's rule, this classification decision is actually owned by `understanding`'s `ClassifyAndRouteMessage` command handler, not duplicated in `ingestion` — this document's Section 7 describes the heuristic *content*, while `10_backend_blueprint.md` Section 4's `understanding` module owns the actual handler).
2. **Transactional-content heuristic (for non-known senders):** A lightweight, deterministic classifier evaluates: presence of currency symbols/amount-like patterns in the subject/snippet, presence of transaction-suggestive keywords ("receipt," "payment," "charged," "transferred," "invoice," localized equivalents), and absence of clear non-transactional markers (newsletter unsubscribe footers, marketing-campaign headers like `List-Unsubscribe`).
3. **Routing decision:**
   - Known sender, high pattern-match confidence → `template_parser`.
   - Unknown sender, positive transactional heuristic → `ai_extractor`.
   - Unknown sender, ambiguous heuristic (neither clearly positive nor negative) → `ai_extractor` (favoring recall per `03_software_requirement_specification.md` Section 4.2's explicit SRS-FR-GMAIL-04 error-handling rule: false negatives are worse than a wasted AI call).
   - Clearly non-transactional (strong negative signal, e.g., a `List-Unsubscribe` header combined with no currency-like content) → `discarded`, logged with a reason code, never enqueued for extraction.

### 7.2 Why This Heuristic Lives Partly at the Connector Layer

While the final classification *decision* is owned by the `understanding` module (per the module-boundary rule), the **initial full-sync query filter** (Section 6.3) necessarily embeds a lighter-weight version of this heuristic at the Gmail API query level, purely as a **pre-filtering optimization** to avoid enqueueing obviously irrelevant mail (e.g., a user's mailbox may contain tens of thousands of non-financial emails within a 90-day window). This is explicitly documented as an optimization, not a duplicated source of truth — the connector's query-level filter is intentionally permissive (favoring over-inclusion), with the `understanding` module's classification remaining the authoritative decision for every message that does get ingested.

---

## 8. Gmail API Quota Management

- Gmail API enforces per-project and per-user quota units (a fixed daily/per-second budget across all API method calls, weighted by method cost — `messages.get` and `history.list` have different unit costs).
- The connector's outbound API calls are instrumented (OpenTelemetry, per `06_architecture.md` Section 10) with per-account and platform-wide quota-consumption metrics, exposed to Connector Monitoring (`17_observability.md`).
- **429 (rate-limited) responses** are treated as transient failures subject to the standard exponential backoff policy (`06_architecture.md` Section 6.4), with Gmail-API-specific backoff tuning (Google recommends and this connector implements truncated exponential backoff per their published guidance) rather than the platform's generic default backoff curve, since Gmail's rate-limit recovery characteristics are documented and specific.
- The 15-minute poll interval (Section 5.3) and the 90-day initial-sync window (Section 6.3) are the two primary quota-consumption levers **intentionally exposed as configuration**, allowing operations to tune platform-wide quota usage against Google's project-level quota grant as the user base scales — directly feeding `23_scalability.md`'s capacity planning.

---

## 9. Failure Modes Specific to Gmail

| Failure | Detection | Handling |
|---|---|---|
| Watch registration fails (e.g., Pub/Sub topic misconfiguration) | Non-2xx from `users.watch` | Logged as `WatchRenewalFailed`; retried per standard backoff; if persistently failing, the account falls back to **poll-only mode** (Section 5.3's fallback trigger becomes the sole ingestion path) with an elevated-severity Connector Monitoring alert, since poll-only significantly degrades latency but must never fully halt ingestion. |
| `historyId` permanently invalid (very long downtime) | Repeated `404` even after bounded fallback (Section 6.2) | Escalates to a full bounded re-sync (same mechanics as Section 6.3's initial sync, but scoped to the downtime window plus safety margin), logged distinctly from the routine fallback case for Connector Monitoring visibility. |
| Mailbox migrated/deleted (rare) | Persistent `404`/`410`-class errors across all endpoints for an account | Account transitions to `error` status (distinct from `disconnected`, since this wasn't a deliberate user action), surfaced to the user for manual reconnection or removal. |
| Malformed/non-standard MIME structure in a specific message | Parsing exception during message content extraction (pre-classification) | That specific message is marked `processing_status = failed` in `ingested_messages` (per `08_database_design.md` Section 8.2), does not block processing of subsequent messages, and is visible in Parser Monitoring as a distinct failure category from template/AI extraction failures. |

---

## 10. Detailed Explanation: Why History API Over Repeated Full Polling

An alternative, simpler design would periodically re-run `messages.list` with a recent-date filter instead of using the History API's delta mechanism. This connector explicitly uses the History API because:

1. **Correctness:** `messages.list` with a date filter can miss messages that arrive with backdated timestamps or are affected by Gmail's own indexing delays, and provides no reliable way to detect "have I already seen this message" other than the message ID dedup check — which still works, but requires scanning a much larger candidate set repeatedly, wasting quota.
2. **Quota efficiency:** `history.list` returns only the delta since a cursor, at a fraction of the quota cost of repeatedly re-listing a recent time window, especially as the "recent window" needed to be safely overlapping (to avoid missing anything) grows.
3. **This is also why the poll-fallback (Section 5.3) itself calls `FetchIncremental` (History-API-based), not a raw `messages.list` scan** — the "safety net" trigger and the "push-triggered" trigger use the *identical* underlying mechanism, just triggered differently. This symmetry is a deliberate design simplification: there is only one incremental-fetch code path to test and reason about, regardless of what triggered it.

---

## 11. Component Responsibilities

| Component | Responsibility |
|---|---|
| `gmailclient.go` (adapter) | All direct Gmail API interaction: OAuth token usage, Watch registration, History/full-sync calls, quota-aware backoff |
| `ingestion` application layer | Connector-agnostic orchestration: scheduling triggers, persisting sync state, emitting `MessageIngested` |
| Pub/Sub webhook handler | Signature verification and fast enqueueing of the incremental-sync trigger |
| Scheduled poll-fallback task | The H2-resolving safety net, running unconditionally per account |
| `understanding` module (external to this document) | Authoritative sender classification decision, consuming the heuristics described in Section 7 |

---

## 12. Trade-offs

| Decision | Trade-off |
|---|---|
| Shared Pub/Sub topic (not per-user) | Slightly more complex payload-to-account resolution logic at the webhook handler, in exchange for operational scalability to 100,000+ accounts without a proliferation of Google Cloud resources. |
| 15-minute poll-fallback interval (default) | A bounded, non-zero worst-case latency degradation during push outages, in exchange for materially lower Gmail API quota consumption than a tighter interval would require — explicitly tunable, not a fixed trade-off. |
| Permissive (over-inclusive) classification heuristic for ambiguous cases | Some wasted AI-extraction calls on messages that turn out non-transactional, in exchange for near-zero risk of silently missing a real transaction — a direct, deliberate application of the Vision's zero-manual-entry promise, which depends on high recall. |
| Symmetric push/poll-triggered `FetchIncremental` path | Slightly less flexibility to have push and poll behave differently (e.g., different batch sizes), in exchange for a single, simpler, more testable incremental-fetch implementation. |

## 13. Advantages
- Every hotspot and constraint identified in earlier documents (H2, the 7-day watch limit, Gmail quota limits) has a concrete, numbered, tunable resolution here — nothing is left as an implicit assumption at implementation time.
- The Connector interface (Section 3.1) is genuinely reusable — a future Outlook connector implementing the same five methods would slot into the exact same `ingestion` application layer without modification, validating the Vision's extensibility promise concretely.
- The symmetric push/poll design (Section 10) minimizes the surface area that needs independent testing and reasoning.

## 14. Disadvantages
- Reliance on Gmail's History API's retention window being "typically 7-30 days" (not contractually fixed by Google) means the bounded-fallback window (Section 6.2) is a best-effort, empirically-tuned value that could require adjustment if Google's actual retention behavior shifts — an accepted, monitored risk rather than a hard guarantee.
- The permissive classification heuristic (Section 7.1, favoring recall) does mean a non-trivial volume of ultimately-discarded-or-low-value AI extraction calls in absolute terms, a direct cost trade-off that must be weighed against `11_ai_blueprint.md` Section 11's cost governance in production.

## 15. Future Improvements
- Once Outlook/IMAP connectors are prioritized (`24_future_roadmap.md`), extract Section 7's classification heuristic content into a connector-agnostic shared library, since transactional-content heuristics (currency patterns, keyword sets) are largely source-independent, while only the sender-registry matching mechanics are Gmail-specific.
- Investigate Gmail API's `historyTypes=labelAdded` in combination with `messageAdded` if future features (e.g., detecting when a user manually archives/deletes a source email, as a signal) are prioritized — currently explicitly out of scope, but the History API supports it.
- Consider a per-account adaptive poll interval (shorter for users experiencing frequent push failures, longer for reliably-pushing accounts) as a refinement to the current fixed 15-minute default, once sufficient production data exists to justify the added complexity.

## 16. Best Practices
- Any change to the requested OAuth scope set (Section 4.1) requires security review and must be reflected in both this document and `16_security.md` simultaneously — scope changes are never a unilateral connector-team decision.
- The bounded-fallback window (Section 6.2) and poll interval (Section 5.3) must be treated as living, monitored configuration, reviewed at least quarterly against real Gmail API behavior and quota consumption data, not set once at launch and forgotten.
- Every new Gmail API failure mode discovered in production must be added to Section 9's table, keeping this document the living reference for on-call engineers.

## 17. Production Considerations
- Gmail API quota headroom must be provisioned with growth margin ahead of user-base scaling milestones, coordinated with Google Cloud project quota increase requests, which can have non-trivial lead times — this belongs in `23_scalability.md`'s capacity planning calendar.
- The poll-fallback safety net (Section 5.3) must itself be monitored for its own health (is the scheduled task actually running reliably?) — a failure of the safety net itself would silently re-open the H2 gap it exists to close, making its own liveness a first-class Connector Monitoring metric, not an assumed-reliable background process.
- Watch registration failures degrading an account to poll-only mode (Section 9) must have a clear operational runbook and user-facing status indication (`09_api_specification.md`'s `gmail_accounts.status`), since a silently degraded account provides a materially worse experience than the primary design promises.

---

**Next Document:** `13_email_parser.md` — specifies the deterministic Template/Regex Parser in full: `SenderTemplate` authoring and versioning workflow, extraction rule DSL, the template-failure-to-AI-fallback mechanism, and Parser Monitoring's failure-rate-spike detection design.
