# Sprint 3 — Ingestion II: Watch, Incremental Sync, Poll Fallback

**Theme:** Complete the `ingestion` module with real-time Gmail Watch/Pub-Sub push, History-API incremental sync, and the dual-trigger (push+poll) design that resolves Hotspot H2.

**Primary documents:** `12_gmail_connector.md` §5, §6, §8, §9; `06_architecture.md` §6.6; `07_sequence_diagrams.md` Diagram 2

**Entry criteria:** Sprint 2 complete — initial sync and Outbox Relay working.

---

## Checklist

### Gmail Watch
- [ ] `users.watch` registration against a platform-owned Pub/Sub topic (`12_gmail_connector.md` §5.1)
- [ ] Command: `RegisterGmailWatch`
- [ ] Scheduled renewal task: fires at ≤6 days from registration (1-day safety margin) **[GATE]** (`12_gmail_connector.md` §5.2)
- [ ] Renewal is idempotent — re-registering before expiry just extends it

### Pub/Sub Webhook
- [ ] `POST /webhooks/gmail/pubsub` handler (`09_api_specification.md` §6.7)
- [ ] JWT/signature verification **before** enqueueing **[GATE — never process an unverified payload]** (`06_architecture.md` §5.4 / `16_security.md`)
- [ ] Fast `200` response regardless of downstream processing outcome (don't block on processing)
- [ ] Webhook triggers `RunIncrementalSync` enqueue onto `ingestion:critical`

### Incremental Sync (History API)
- [ ] Command: `RunIncrementalSync` using `history.list` from stored `last_history_id`
- [ ] `last_history_id` updated only after full batch durably recorded (transactional) **[GATE]**
- [ ] `404`/expired `historyId` → bounded fallback re-sync (default 3 days), **not** full 90-day re-scan (`12_gmail_connector.md` §6.2)
- [ ] `HistorySyncFallbackTriggered` event logged distinctly for Connector Monitoring

### Poll-Fallback Safety Net (H2 Resolution)
- [ ] Scheduled periodic task, independent of push health, default 15 min interval, per-account (`12_gmail_connector.md` §5.3) **[GATE — this is the single most important item this sprint]**
- [ ] Calls the **same** `RunIncrementalSync` command as the push-triggered path (symmetric design, per `12_gmail_connector.md` §10)
- [ ] Poll interval exposed as tunable config (Viper), not hardcoded
- [ ] Liveness heartbeat metric for the poll-fallback scheduler itself (`17_observability.md` §4.1) **[GATE — "monitor the monitor"]**

### Failure Handling
- [ ] 429 rate-limit handling with Gmail-specific truncated exponential backoff (`12_gmail_connector.md` §8)
- [ ] Watch registration failure → account falls back to poll-only mode, elevated-severity alert (`12_gmail_connector.md` §9)
- [ ] Malformed MIME parsing failure isolated to that one message (`processing_status = failed`), never blocks the batch

### Account Lifecycle
- [ ] Command: `DisconnectGmailAccount` — stops watch, transitions status
- [ ] OAuth revocation detection: `invalid_grant` → `disconnected` status + reconnection-prompt trigger (`16_security.md` §4.3's token-refresh classification)

### Observability
- [ ] Metrics wired: `ingestion_gmail_api_calls_total`, `ingestion_watch_renewal_success/failure_total`, `ingestion_history_sync_fallback_total`, `ingestion_poll_fallback_liveness`, `ingestion_sync_lag_seconds` (`17_observability.md` §4.1)
- [ ] Alert rule configured: poll-fallback liveness heartbeat stale → Critical (`17_observability.md` §8) **[GATE — wire this now, don't defer to Sprint 10]**
- [ ] Trace context propagated from Pub/Sub webhook through to the enqueued `RunIncrementalSync` task (verify end-to-end, not just per-hop)

### Frontend (Parallel Track)
- [ ] Account detail view surfaces `status = disconnected` / `status = error` clearly, with a "Reconnect" CTA wired to re-trigger OAuth linking
- [ ] "Disconnect account" action (`DELETE /me/gmail-accounts/{id}`) with a clear, explicit choice between "keep my data" and "delete my data" (per `FR-USER-02` — do not default silently to either)

### Testing
- [ ] Integration test: push-triggered incremental sync (Diagram 2)
- [ ] Integration test: poll-fallback alone (no push simulated) correctly processes new mail — direct H2 verification **[GATE]**
- [ ] Integration test: `historyId` expiry triggers bounded fallback, not full re-sync

---

## Exit Criteria

- [ ] New transaction email in a connected test mailbox is ingested within the SRS latency target via push
- [ ] With push artificially disabled, the same email is still ingested within one poll interval
- [ ] Watch renewal demonstrably fires before the 7-day Gmail-enforced expiry
- [ ] Poll-fallback liveness metric is visible and alertable
- [ ] Frontend: disconnecting an account in the UI correctly reflects the new status and offers the data-retention choice
