# Sprint 2 — Ingestion I: OAuth Linking & Initial Sync

**Theme:** Stand up the `ingestion` module's onboarding path — linking a Gmail account and performing the bounded initial full sync. Real-time sync (Watch/push) is deliberately deferred to Sprint 3.

**Primary documents:** `12_gmail_connector.md` §3–4, §6.3, §7; `07_sequence_diagrams.md` Diagram 1

**Entry criteria:** Sprint 1 complete — `identity` module's `TokenProvider` interface is available and working.

---

## Checklist

### Connector Abstraction
- [ ] Define the general `Connector` interface (`Authenticate`, `InitialSync`, `RegisterRealtimeWatch`, `FetchIncremental`, `Disconnect`) in `ingestion/domain/connector.go` **[GATE — this is the abstraction every future connector must satisfy, get the shape right now]** (`12_gmail_connector.md` §3.1)

### Ingestion Module — Domain Layer
- [ ] `internal/ingestion/domain/gmail_account.go` — `GmailAccount` aggregate incl. sync state
- [ ] `internal/ingestion/domain/ingested_message.go` — `IngestedMessage` entity
- [ ] `internal/ingestion/domain/repository.go`
- [ ] `internal/ingestion/domain/event.go` — `GmailAccountLinked`, `MessageIngested`, etc.

### Cross-Module Wiring
- [ ] Consume `identity.TokenProvider` interface (per the sanctioned cross-module interface table, `10_backend_blueprint.md` §6) — verify this is the **only** way `ingestion` touches `identity`

### Gmail OAuth Linking
- [ ] Command: `LinkGmailAccount`
- [ ] `adapter/http/handler.go`: `POST /me/gmail-accounts` (`09_api_specification.md` §6.1)
- [ ] `UNIQUE (email_address)` constraint verified enforced at DB level — duplicate linking returns `409` **[GATE, re-verify from Sprint 1]**

### Initial Full Sync
- [ ] `adapter/external/gmailclient.go`: `messages.list` with query filter (known-sender OR keyword heuristic) (`12_gmail_connector.md` §6.3, §7.1)
- [ ] Command: `StartInitialFullSync` — bounded to configurable lookback window (default 90 days)
- [ ] Pagination handling: persist last successful page token, resumable after downtime
- [ ] Transactional per-page: `INSERT ingested_messages` (unique on `gmail_message_id`) + `INSERT outbox_events(MessageIngested)` in same transaction **[GATE]**
- [ ] On completion: store final `historyId`, transition account `initial_sync` → `active`

### Outbox Relay
- [ ] Implement `internal/platform/outboxrelay/` generic poller (`06_architecture.md` §6.1) — `SELECT ... FOR UPDATE SKIP LOCKED`
- [ ] Instantiate in `ingestion/adapter/outbox/relay.go`
- [ ] Asynq queue topology stood up: `ingestion:critical`, `ingestion:bulk` (`06_architecture.md` §6.3)

### Sender Classification Heuristic (Initial Version)
- [ ] Transactional-content heuristic implemented (currency patterns, keyword sets, `List-Unsubscribe` negative signal) (`12_gmail_connector.md` §7.1)
- [ ] Permissive-on-ambiguity rule implemented: ambiguous → `ai_extractor` routing, never silent discard **[GATE]**

### Testing
- [ ] Unit tests: sender classification heuristic against a fixture set of sample subject/snippet pairs
- [ ] Integration test: full onboarding flow (Diagram 1) — link account, trigger initial sync, verify `MessageIngested` events land in Outbox
- [ ] Integration test: resumed sync after simulated mid-pagination crash continues correctly, no duplicate/skipped messages

### Frontend (Parallel Track)
- [ ] "Connect Gmail" screen: triggers `POST /me/gmail-accounts`, redirects through OAuth consent
- [ ] Linked-accounts list view: `GET /me/gmail-accounts`, shows `status` (`linking`/`initial_sync`/`active`/`disconnected`/`error`)
- [ ] "Syncing..." state UI with simple progress indication while `status = initial_sync` (poll `GET /me/gmail-accounts` on an interval — no websocket needed at MVP)
- [ ] Empty-state messaging for a newly-linked account with zero events yet

### Observability
- [ ] Metrics wired: `ingestion_gmail_api_calls_total`, `ingestion_messages_ingested_total` (`17_observability.md` §4.1)
- [ ] Trace spans on every outbound Gmail API call, tagged with `external_system=gmail`
- [ ] Frontend: track and report the "connect Gmail" funnel (initiated → consent completed → first sync completed) as a basic analytics/error-tracking event, to catch onboarding drop-off early

---

## Exit Criteria

- [ ] A real (or sandboxed test) Gmail account can be linked and its last-90-days transactional mail enumerated into `ingested_messages`
- [ ] Outbox Relay demonstrably delivers `MessageIngested` events into Asynq queues
- [ ] No duplicate `ingested_messages` rows under simulated redelivery
- [ ] `ingestion` module passes full test suite; module-boundary linter confirms only the sanctioned `TokenProvider` cross-module import exists
- [ ] Frontend: a user can click through the full "Connect Gmail" flow in a browser and see the account transition from "syncing" to "active"
