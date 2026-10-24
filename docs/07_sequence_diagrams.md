# Sequence Diagrams
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `05_event_storming.md`, `06_architecture.md`
**Feeds Into:** `08_database_design.md`, `09_api_specification.md`, `10_backend_blueprint.md`, `18_testing_strategy.md`

---

## 1. Purpose

This document expands the highest-value process flows from `05_event_storming.md` into detailed, component-level sequence diagrams, grounded in the concrete technical components defined in `06_architecture.md` (chi handlers, Asynq queues, Outbox/Inbox tables, PostgreSQL, Redis, external APIs). Where the Event Storming document shows *what* happens in domain terms, this document shows *how* it happens across actual system components — including transaction boundaries, retries, and failure branches.

These diagrams are the direct input for:
- `08_database_design.md` — every table touched in a transaction boundary here must exist in the schema.
- `09_api_specification.md` — every synchronous request/response shown here becomes an endpoint contract.
- `18_testing_strategy.md` — each diagram is a candidate integration/E2E test scenario, including its failure branches.

## 2. Scope

### 2.1 In Scope
Eight sequence diagrams covering:
1. Gmail account onboarding (OAuth linking + initial sync trigger)
2. Real-time ingestion via Gmail Push → incremental sync
3. Known-sender template extraction → finalization (happy path)
4. Unknown-sender AI extraction with fallback and low-confidence review path
5. Duplicate delivery handling (Inbox pattern in action)
6. Budget threshold evaluation and alert firing
7. Notification composition and WhatsApp delivery (with retry)
8. Account disconnection and data purge cascade

### 2.2 Out of Scope
- Full CRUD flows for simple settings management (trivial, covered directly in `09_api_specification.md`).
- UI-level interaction detail (this is a backend blueprint series; frontend flows are out of scope per the original brief).

---

## 3. Diagram Notation

Diagrams use a text-based sequence format: `Actor -> Component: Message` for synchronous calls, `Actor --> Component: Message` for asynchronous/queued messages, `Component -->> Actor: Response` for returns, and `[TX: ...]` blocks to denote explicit database transaction boundaries — this is the detail that matters most for correctness review in a financial system.

---

## 4. Diagram 1 — Gmail Account Onboarding

**Traces to:** `05_event_storming.md` Phase 1; `FR-AUTH-01`, `FR-GMAIL-01/02`.

```
User          chi API          identity module      Google OAuth      ingestion module        Asynq/Redis
 |               |                    |                    |                   |                    |
 |--Sign in----->|                    |                    |                   |                    |
 |               |--InitiateOAuth---->|                    |                   |                    |
 |               |                    |--redirect (PKCE)-->|                   |                    |
 |<--302 redirect to Google-----------|                    |                   |                    |
 |----consent-------------------------------------------->|                   |                    |
 |<--redirect w/ auth code-------------------------------- |                   |                    |
 |--callback(code)->|                 |                    |                   |                    |
 |               |--ExchangeCode----->|                    |                   |                    |
 |               |                    |--token exchange--->|                   |                    |
 |               |                    |<--access+refresh---|                   |                    |
 |               |   [TX: identity]                        |                   |                    |
 |               |    INSERT user (if new)                 |                   |                    |
 |               |    INSERT oauth_token (encrypted)        |                   |                    |
 |               |    INSERT outbox_events(UserSignedIn)     |                   |                    |
 |               |   [COMMIT]                               |                   |                    |
 |               |--LinkGmailAccount->|                    |                   |                    |
 |               |   [TX: ingestion]                        |                   |                    |
 |               |    INSERT gmail_accounts (status=linking) |                   |                    |
 |               |    INSERT outbox_events(GmailAccountLinked)|                   |                    |
 |               |   [COMMIT]                               |                   |                    |
 |<--200 OK (account linking)---------|                    |                   |                    |
 |               |                    |                    |     (Outbox Relay polls) ------------->|
 |               |                    |                    |                   |--enqueue StartInitialFullSync-->|
 |               |                    |                    |                   |--enqueue RegisterGmailWatch---->|
```

**Notes:**
- The user-facing HTTP response returns immediately after the linking record and Outbox rows commit — the actual Gmail sync and watch registration happen asynchronously, keeping the user-facing latency low (satisfies SRS 6.1 implicitly by not blocking the HTTP response on external Gmail API calls).
- `H1` (duplicate Gmail linking, from `05_event_storming.md`) is enforced at the `INSERT gmail_accounts` step via a unique constraint on `email_address`; a violation here returns a `409 Conflict` to the API layer rather than proceeding.

---

## 5. Diagram 2 — Real-Time Ingestion via Gmail Push

**Traces to:** `05_event_storming.md` Phase 2; resolves H2 (dual-trigger).

```
Google Pub/Sub     chi API (webhook)   ingestion worker      Gmail API         Asynq/Redis
      |                   |                    |                  |                  |
      |--push notif------>|                    |                  |                  |
      |                   |--verify JWT-------->|                  |                  |
      |                   |--enqueue(ingestion:critical)---------------------------->|
      |<--200 OK----------|                    |                  |                  |
      |                   |                    |<--dequeue RunIncrementalSync---------|
      |                   |                    |--history.list(historyId)-->|
      |                   |                    |<--delta messages-----------|
      |                   |                    |  [TX: ingestion]            |
      |                   |                    |   for each new message:      |
      |                   |                    |     INSERT ingested_messages  |
      |                   |                    |       (unique on gmail_message_id)
      |                   |                    |     INSERT outbox_events(MessageIngested)
      |                   |                    |   UPDATE sync_state.last_history_id
      |                   |                    |  [COMMIT]                    |
      |                   |                    |                              |
      | -- (independently, every 15 min, regardless of push health) --        |
      |                   |                    |<--dequeue RunIncrementalSync (scheduled)--|
      |                   |                    |  (same idempotent flow; typically no-op) |
```

**Notes:**
- Webhook signature/JWT verification happens **before** enqueueing, per `03_software_requirement_specification.md` Section 5's Pub/Sub interface requirement — an unverified payload is rejected with `401` and never reaches a queue.
- The `historyId` update only commits after all new messages in the batch are durably recorded, ensuring a crash mid-batch causes safe re-processing of the same batch on retry (idempotent via the `gmail_message_id` unique constraint), not a skipped gap.
- The scheduled poll-fallback branch (bottom) is the concrete H2 resolution — it runs unconditionally on a fixed interval per `gmail_account`, independent of whether push notifications have been arriving.

---

## 6. Diagram 3 — Known-Sender Template Extraction (Happy Path)

**Traces to:** `05_event_storming.md` Phases 3–7; `FR-PARSE-01`, `FR-AI-03`, `FR-ANALYTICS-*`, `FR-BUDGET-*`.

```
Asynq(understanding:template)   understanding module   financialcore module   analytics module   budget module
            |                            |                       |                     |                 |
   dequeue ClassifyAndRouteMessage        |                       |                     |                 |
            |-------------------------->|                       |                     |                 |
            |   lookup known_sender_registry (Redis cache)        |                     |                 |
            |   route = template_parser                            |                     |                 |
            |   enqueue ApplySenderTemplate ------------------------------------------------------------->|
            |                            |                       |                     |                 |
   dequeue ApplySenderTemplate             |                       |                     |                 |
            |-------------------------->|                       |                     |                 |
            |  apply SenderTemplate v3    |                       |                     |                 |
            |  fields fully matched        |                       |                     |                 |
            |  [TX: understanding]          |                       |                     |                 |
            |   INSERT extraction_results (method=template, confidence=1.0)              |                 |
            |   INSERT outbox_events(FinancialEventCandidateCreated)                       |                 |
            |  [COMMIT]                     |                       |                     |                 |
            |                            |<--dequeue EvaluateConfidenceGate--             |                 |
            |                            |  confidence 1.0 >= threshold 0.70               |                 |
            |                            |  [TX: financialcore]                            |                 |
            |                            |   INSERT financial_events (status=finalized,     |                 |
            |                            |     UNIQUE(connector_account_id, source_message_ref)) |           |
            |                            |   INSERT outbox_events(FinancialEventFinalized)   |                 |
            |                            |  [COMMIT]                                        |                 |
            |                            |   enqueue AssignCategory ------------------------------------->|
            |                            |                       |  lookup user_history / global_mapping     |
            |                            |                       |  [TX: financialcore]                       |
            |                            |                       |   UPDATE financial_events.category_assignment
            |                            |                       |   INSERT outbox_events(CategoryAssignmentChanged)
            |                            |                       |  [COMMIT]                                  |
            |                            |                       |     enqueue ProjectAnalyticsReadModels --->|
            |                            |                       |     enqueue EvaluateBudgetImpact -------------------->|
            |                            |                       |                     |  [TX: analytics]     |
            |                            |                       |                     |   UPSERT cash_flow_view, category_breakdown, trend_view
            |                            |                       |                     |  [COMMIT]             |
            |                            |                       |                     |                 |  [TX: budget]
            |                            |                       |                     |                 |   check fired_alerts, evaluate threshold
            |                            |                       |                     |                 |   (no new threshold crossed in this example)
            |                            |                       |                     |                 |  [COMMIT, no event emitted]
```

**Notes:**
- The categorize-before/after-gating ordering follows `05_event_storming.md`'s H5 resolution: categorization happens **after** finalization here because this is the fully-confident happy path; the candidate-time categorization for pending-review items is shown separately in Diagram 4.
- Every arrow crossing a queue boundary corresponds 1:1 to a named Policy in `05_event_storming.md` — this traceability is intentional and must be preserved as the system evolves.

---

## 7. Diagram 4 — Unknown-Sender AI Extraction with Low-Confidence Review Path

**Traces to:** `05_event_storming.md` Phase 3–4; `FR-AI-01/02/03`; Hotspot H3, H4.

```
understanding module        LLM Provider        financialcore module      engagement module        User (WhatsApp)
        |                        |                       |                        |                       |
 dequeue InvokeAIExtractor        |                       |                        |                       |
        |--structured extraction request (prompt v7)---->|                        |                       |
        |<--JSON response----------------------------------|                        |                       |
        |  schema-validate: OK                              |                        |                       |
        |  [TX: understanding]                               |                        |                       |
        |   INSERT extraction_results (method=ai, prompt_version=7,                   |                       |
        |     per-field confidence, aggregate confidence=0.58)                        |                       |
        |   INSERT outbox_events(FinancialEventCandidateCreated)                       |                       |
        |  [COMMIT]                                           |                        |                       |
        |   enqueue AssignCategory (candidate-time, per H5)---->|                        |                       |
        |                        |                       |  category suggested (ai_inference, conf 0.6)          |
        |                        |                       |   enqueue EvaluateConfidenceGate                     |
        |                        |                       |  [TX: financialcore]                                  |
        |                        |                       |   confidence 0.58 < threshold 0.70                     |
        |                        |                       |   INSERT financial_events (status=pending_review)      |
        |                        |                       |   INSERT outbox_events(FinancialEventPendingReview)    |
        |                        |                       |  [COMMIT]                                              |
        |                        |                       |   enqueue ComposeReviewRequestNotification ----------->|
        |                        |                       |                        |  [TX: engagement]            |
        |                        |                       |                        |   INSERT notification_jobs    |
        |                        |                       |                        |   render approved WhatsApp     |
        |                        |                       |                        |   template w/ suggested fields |
        |                        |                       |                        |  [COMMIT]                       |
        |                        |                       |                        |   enqueue DeliverViaWhatsApp -->|--"We detected a $45
        |                        |                       |                        |                                 | charge from 'AMZN Mktp'
        |                        |                       |                        |                                 | categorized as Shopping.
        |                        |                       |                        |                                 | Reply Y to confirm or
        |                        |                       |                        |                                 | tap Edit."---------------->|
        |                        |                       |                        |                                 |<--User replies "Y"--------|
        |                        |                       |                        |  [parse structured reply -- H4]   |
        |                        |                       |<--ConfirmPendingReviewEvent command----------------------|
        |                        |                       |  [TX: financialcore]                                    |
        |                        |                       |   UPDATE financial_events SET status=finalized           |
        |                        |                       |   INSERT outbox_events(FinancialEventFinalized)          |
        |                        |                       |  [COMMIT]                                                |
        |                        |                       |   (fan-out to Analytics/Budget as in Diagram 3)          |
```

**Notes on H4 resolution:** The WhatsApp reply-parsing step is deliberately shown as a distinct, bounded sub-step ("parse structured reply"), not a generic free-text NLU problem. The Engagement module constrains the interaction to a small, closed intent set (`Y` / `N` / `Edit <field>`) enforced via WhatsApp's interactive button/quick-reply features where the provider supports them, falling back to strict text pattern-matching otherwise — full design in `15_notification_system.md`. This keeps the confirmation flow reliable without requiring a second AI extraction pass just to confirm the first one.

---

## 8. Diagram 5 — Duplicate Delivery Handling (Inbox Pattern)

**Traces to:** `06_architecture.md` Section 6.2; `SRS-FR-PARSE-02`; INV-3.

```
Asynq (redelivery scenario)     ingestion worker          PostgreSQL (financialcore schema)
         |                             |                            |
  dequeue ClassifyAndRouteMessage (attempt 1)                        |
         |---------------------------->|                            |
         |   ... processing proceeds through Understanding ...       |
         |   worker crashes AFTER finalizing but BEFORE Asynq ack    |
         |                             |                            |
  Asynq redelivers same task (attempt 2, at-least-once semantics)     |
         |---------------------------->|                            |
         |   re-runs EvaluateConfidenceGate -> attempts finalization  |
         |   [TX: financialcore]                                      |
         |    INSERT financial_events (...)                            |
         |      ON CONFLICT (connector_account_id, source_message_ref)  |
         |      DO NOTHING  -- INV-3 unique constraint                    |
         |   [COMMIT]  -- treated as successful no-op, not an error        |
         |   task acked as successful                                       |
```

**Notes:**
- This diagram demonstrates the core correctness guarantee promised in `06_architecture.md`: at-least-once transport (Asynq redelivery) combined with a domain-level uniqueness invariant (INV-3) produces exactly-once *effect* on the ledger, without requiring perfect exactly-once delivery infrastructure.
- The same pattern applies identically to `IngestedMessage` creation (unique on `gmail_message_id`) earlier in the pipeline (Diagram 2) — duplicate protection exists at **every** stage a duplicate could plausibly reappear, not only at the final ledger write.

---

## 9. Diagram 6 — Budget Threshold Evaluation and Alert Firing

**Traces to:** `05_event_storming.md` Phase 7; `FR-BUDGET-02`.

```
financialcore module        budget module              engagement module         User (WhatsApp)
        |                        |                              |                       |
  FinancialEventFinalized (event_type=Expense, amount=$120)       |                       |
        |----------------------->|                              |                       |
        |   [TX: budget]                                          |                       |
        |    SELECT budget WHERE scope matches category            |                       |
        |    recompute cumulative spend for current period          |                       |
        |    cumulative = $410 of $500 budget = 82%                  |                       |
        |    check fired_alerts for (period, threshold=0.8)           |                       |
        |    -- not yet fired --                                       |                       |
        |    INSERT INTO fired_alerts (period, threshold=0.8)           |                       |
        |    INSERT outbox_events(BudgetThresholdCrossed)                |                       |
        |   [COMMIT]                                                     |                       |
        |                        |   enqueue ComposeBudgetAlertNotification-->|                       |
        |                        |                              |  [TX: engagement]        |
        |                        |                              |   render approved template |
        |                        |                              |   "You've used 82% of your |
        |                        |                              |    Dining budget this month"|
        |                        |                              |  [COMMIT]                   |
        |                        |                              |   enqueue DeliverViaWhatsApp-->|--(delivered)-->|
```

**Notes:** The idempotent `fired_alerts` check means that if this same `FinancialEventFinalized` event were somehow re-delivered (Diagram 5's scenario applied here), the budget module would recompute the same 82% figure but find the `(period, 0.8)` pair already fired, and correctly suppress a duplicate alert — this is the concrete mechanism satisfying the "single notification per threshold per period" requirement from the SRS.

---

## 10. Diagram 7 — Notification Delivery with Retry

**Traces to:** `05_event_storming.md` Phase 8; `FR-NOTIF-05`.

```
engagement worker         WhatsApp Business API        Asynq (engagement:deliver queue)
       |                          |                              |
 dequeue DeliverViaWhatsApp (attempt 1)                            |
       |------------------------>|                              |
       |<--503 Service Unavailable|                              |
       |  classify as transient    |                              |
       |  [TX: engagement]           |                              |
       |   UPDATE notification_jobs SET delivery_status=retrying    |
       |  [COMMIT]                    |                              |
       |   task NACKed -- Asynq schedules retry (backoff: 10s)         |
       |                          |                              |
 dequeue DeliverViaWhatsApp (attempt 2, after backoff)                |
       |------------------------>|                              |
       |<--200 OK (message queued by provider)                     |
       |  [TX: engagement]           |                              |
       |   UPDATE notification_jobs SET delivery_status=delivered    |
       |   INSERT outbox_events(NotificationDelivered)                 |
       |  [COMMIT]                    |                              |
       |                          |                              |
 (later) WhatsApp delivery webhook confirms end-user receipt --------->|
       |  [TX: engagement]           |                              |
       |   UPDATE notification_jobs SET delivery_confirmed_at=now()    |
       |  [COMMIT]                    |                              |
```

**Notes:** Two distinct confirmation levels are modeled: "accepted by provider" (200 OK from the send API call) vs. "confirmed delivered to end user" (async webhook). Administration dashboards (`17_observability.md`) track both, since a provider-accepted message can still fail delivery downstream (e.g., invalid number) — that failure surfaces via the webhook path, not the initial send call.

---

## 11. Diagram 8 — Account Disconnection and Data Purge Cascade

**Traces to:** `05_event_storming.md` Phase 9; Hotspot H8.

```
User        chi API        ingestion module      financialcore module    analytics module    administration module
 |             |                   |                        |                     |                     |
 |--Disconnect+Purge------------->|                        |                     |                     |
 |             |--DisconnectGmailAccount-->|                |                     |                     |
 |             |  [TX: ingestion]           |                |                     |                     |
 |             |   UPDATE gmail_accounts SET status=disconnected                    |                     |
 |             |   INSERT outbox_events(AccountDisconnected)                        |                     |
 |             |  [COMMIT]                    |                |                     |                     |
 |             |   enqueue StopGmailWatch (calls Gmail API stop, best-effort)         |                     |
 |             |--RequestDataPurge-------->|                |                     |                     |
 |             |  [TX: ingestion]           |                |                     |                     |
 |             |   INSERT outbox_events(UserDataPurgeRequested)                       |                     |
 |             |  [COMMIT]                    |                |                     |                     |
 |<--202 Accepted (purge queued)---|                        |                     |                     |
 |             |                   |   enqueue ArchiveFinancialEvents ------------->|                     |
 |             |                   |                        |  [TX: financialcore]  |                     |
 |             |                   |                        |   UPDATE financial_events                    |
 |             |                   |                        |     SET status=archived                       |
 |             |                   |                        |     WHERE connector_account_id = ?              |
 |             |                   |                        |   INSERT outbox_events(FinancialEventArchived) x N |
 |             |                   |                        |  [COMMIT]                                          |
 |             |                   |                        |    enqueue ProjectAnalyticsReadModels (exclude) -->|
 |             |                   |                        |                     |  [TX: analytics]      |
 |             |                   |                        |                     |   recompute/exclude archived events from all views
 |             |                   |                        |                     |  [COMMIT]              |
 |             |                   |                        |    every archival event also fans to ---------------------------->|
 |             |                   |                        |                     |                     |  [TX: administration]
 |             |                   |                        |                     |                     |   append-only INSERT audit_log
 |             |                   |                        |                     |                     |   (records the purge action itself;
 |             |                   |                        |                     |                     |    this audit record is explicitly
 |             |                   |                        |                     |                     |    EXEMPT from later erasure)
 |             |                   |                        |                     |                     |  [COMMIT]
 |
 | -- (scheduled sweep, after retention window elapses) --
 |             |                   |                        |  PermanentlyEraseFinancialEvent (batch)      |
 |             |                   |                        |   DELETE FROM financial_events WHERE archived_at < now() - retention_window
```

**Notes on H8 resolution:** The cascade is shown explicitly touching four modules (ingestion → financialcore → analytics → administration), with the audit log insertion **explicitly called out as exempt** from the later permanent-erasure sweep — this is the precise mechanism that prevents the compliance failure mode flagged in `05_event_storming.md` (a GDPR-compliant deletion must be provable after the fact, which requires the deletion event itself to survive even though the underlying data does not). `18_testing_strategy.md` must include an explicit integration test asserting this exact ordering and exemption.

---

## 12. Component Responsibilities (as Demonstrated Across Diagrams)

| Component | Demonstrated Responsibility |
|---|---|
| chi API layer | Fast, non-blocking request acceptance; defers heavy work to async queues (Diagrams 1, 11) |
| Outbox Relay | The universal bridge from every `[TX: ...]` commit to the next queued step (implicit in every diagram, elaborated in `06_architecture.md`) |
| Understanding module | Symmetric candidate production regardless of template vs. AI path (Diagrams 3, 4) |
| Financial Core | Sole writer of `financial_events`; enforces INV-3 at the database layer (Diagram 5) |
| Budget module | Idempotent, replay-safe threshold evaluation (Diagram 6) |
| Engagement module | Two-tier delivery confirmation (provider-accepted vs. user-confirmed) and constrained reply parsing (Diagrams 4, 7) |
| Administration module | Append-only, exemption-aware audit trail across all cascades (Diagram 8) |

---

## 13. Trade-offs

| Decision | Trade-off |
|---|---|
| Async response for onboarding/purge (202/200 + background processing) vs. synchronous completion | Better perceived responsiveness and resilience to slow external APIs, at the cost of needing the client (or a webhook/polling status endpoint, per `09_api_specification.md`) to check completion status rather than getting it in the initial response. |
| Candidate-time categorization even for pending-review items (Diagram 4) | Slightly more AI/lookup work performed on items that may never be confirmed, in exchange for a materially better review UX (per H5's resolution in `05_event_storming.md`). |
| Constrained-intent WhatsApp reply parsing (Y/N/Edit) rather than full NLU | Less flexible for the user (can't freely describe a correction in prose at this stage), in exchange for reliability and avoiding a second, harder AI-parsing problem in the critical confirmation path. |

## 14. Advantages
- Every diagram explicitly marks transaction boundaries, making it trivial for engineers to identify exactly what must be atomic — directly reducing the risk of subtle consistency bugs during implementation.
- The diagrams collectively demonstrate all eight hotspots' resolutions in concrete, testable form, closing the loop opened in `05_event_storming.md`.
- Each diagram is a ready-made integration test scenario outline for `18_testing_strategy.md`.

## 15. Disadvantages
- Text-based sequence notation, while precise, is less immediately visual than a rendered UML sequence diagram — teams preferring visual tooling should consider regenerating these as PlantUML/Mermaid diagrams for onboarding materials (noted as a Future Improvement).
- These diagrams necessarily show only the happy-path-plus-one-failure-branch per scenario for readability; the full failure-branch matrix (all classified error types per external call) is deferred to `18_testing_strategy.md`'s detailed test case enumeration.

## 16. Future Improvements
- Convert these text sequence diagrams into rendered Mermaid/PlantUML diagrams embedded in project documentation tooling once the flows are stable, for easier onboarding of new engineers.
- Add a ninth diagram covering the future "Conversational Query" flow once Phase 3 (Advisory Intelligence) work begins.
- Add explicit diagrams for multi-Gmail-account merge scenarios (Phase 2 per `02_product_requirement_document.md`) once that feature is scheduled.

## 17. Best Practices
- Any new asynchronous flow added to the system must be documented in this file's format (explicit `[TX: ...]` boundaries, explicit queue-crossing arrows) before implementation, mirroring the discipline established in `05_event_storming.md`.
- Every diagram's failure branch shown here must have a corresponding automated test in `18_testing_strategy.md` — a diagram without a test is considered an unverified design claim.

## 18. Production Considerations
- The retry/backoff timings shown illustratively in Diagram 7 (10s) must match the actual configured values from `06_architecture.md` Section 6.4 in both documentation and code — a documentation/code drift check should be part of release review.
- The purge cascade (Diagram 8) must be exercised in a staging environment with production-like data volume before launch, since a slow or partially-failing cascade at scale is a direct GDPR compliance risk, not merely a performance concern.

---

**Next Document:** `08_database_design.md` — translates every entity, invariant, transaction boundary, and outbox/inbox table referenced across these diagrams into concrete PostgreSQL schema definitions, indexes, and constraints, organized per the module-schema-ownership model from `06_architecture.md`.
