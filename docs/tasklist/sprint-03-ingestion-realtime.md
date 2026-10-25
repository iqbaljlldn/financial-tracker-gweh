# Sprint 3 — Ingestion II: Watch, Incremental Sync, Poll Fallback

**Theme:** Complete the `ingestion` module with real-time Gmail Watch/Pub-Sub push, History-API incremental sync, and the dual-trigger (push+poll) design that resolves Hotspot H2.

**Primary documents:** `12_gmail_connector.md` §5, §6, §8, §9; `06_architecture.md` §6.6; `07_sequence_diagrams.md` Diagram 2

**Entry criteria:** Sprint 2 complete — initial sync and Outbox Relay working.

---

## Checklist

### Gmail Watch
- [x] `users.watch` registration against a platform-owned Pub/Sub topic (`12_gmail_connector.md` §5.1)
- [x] Command: `RegisterGmailWatch`
- [x] Scheduled renewal task: fires at ≤6 days from registration (1-day safety margin) **[GATE]** (`12_gmail_connector.md` §5.2)
- [x] Renewal is idempotent — re-registering before expiry just extends it

### Pub/Sub Webhook
- [x] `POST /webhooks/gmail/pubsub` handler (`09_api_specification.md` §6.7)
- [x] JWT/signature verification **before** enqueueing **[GATE — never process an unverified payload]** (`06_architecture.md` §5.4 / `16_security.md`)
- [x] Fast `200` response regardless of downstream processing outcome (don't block on processing)
- [x] Webhook triggers `RunIncrementalSync` enqueue onto `ingestion:critical`

### Incremental Sync (History API)
- [x] Command: `RunIncrementalSync` using `history.list` from stored `last_history_id`
- [x] `last_history_id` updated only after full batch durably recorded (transactional) **[GATE]**
- [x] `404`/expired `historyId` → bounded fallback re-sync (default 3 days), **not** full 90-day re-scan (`12_gmail_connector.md` §6.2)
- [x] `HistorySyncFallbackTriggered` event logged distinctly for Connector Monitoring

### Poll-Fallback Safety Net (H2 Resolution)
- [x] Scheduled periodic task, independent of push health, default 15 min interval, per-account (`12_gmail_connector.md` §5.3) **[GATE — this is the single most important item this sprint]**
- [x] Calls the **same** `RunIncrementalSync` command as the push-triggered path (symmetric design, per `12_gmail_connector.md` §10)
- [x] Poll interval exposed as tunable config (Viper), not hardcoded
- [x] Liveness heartbeat metric for the poll-fallback scheduler itself (`17_observability.md` §4.1) **[GATE — "monitor the monitor"]**

### Failure Handling
- [x] 429 rate-limit handling with Gmail-specific truncated exponential backoff (`12_gmail_connector.md` §8)
- [x] Watch registration failure → account falls back to poll-only mode, elevated-severity alert (`12_gmail_connector.md` §9)
- [x] Malformed MIME parsing failure isolated to that one message (`processing_status = failed`), never blocks the batch

### Account Lifecycle
- [x] Command: `DisconnectGmailAccount` — stops watch, transitions status
- [x] OAuth revocation detection: `invalid_grant` → `disconnected` status + reconnection-prompt trigger (`16_security.md` §4.3's token-refresh classification)

### Observability
- [x] Metrics wired: `ingestion_gmail_api_calls_total`, `ingestion_watch_renewal_success/failure_total`, `ingestion_history_sync_fallback_total`, `ingestion_poll_fallback_liveness`, `ingestion_sync_lag_seconds` (`17_observability.md` §4.1)
- [x] Alert rule configured: poll-fallback liveness heartbeat stale → Critical (`17_observability.md` §8) **[GATE — wire this now, don't defer to Sprint 10]**
- [x] Trace context propagated from Pub/Sub webhook through to the enqueued `RunIncrementalSync` task (verify end-to-end, not just per-hop)

### Frontend (Parallel Track)
- [x] Account detail view surfaces `status = disconnected` / `status = error` clearly, with a "Reconnect" CTA wired to re-trigger OAuth linking
- [x] "Disconnect account" action (`DELETE /me/gmail-accounts/{id}`) with a clear, explicit choice between "keep my data" and "delete my data" (per `FR-USER-02` — do not default silently to either)

### Testing
- [x] Integration test: push-triggered incremental sync (Diagram 2)
- [x] Integration test: poll-fallback alone (no push simulated) correctly processes new mail — direct H2 verification **[GATE]**
- [x] Integration test: `historyId` expiry triggers bounded fallback, not full re-sync

---

## Exit Criteria

- [x] New transaction email in a connected test mailbox is ingested within the SRS latency target via push
- [x] With push artificially disabled, the same email is still ingested within one poll interval
- [x] Watch renewal demonstrably fires before the 7-day Gmail-enforced expiry
- [x] Poll-fallback liveness metric is visible and alertable
- [x] Frontend: disconnecting an account in the UI correctly reflects the new status and offers the data-retention choice
