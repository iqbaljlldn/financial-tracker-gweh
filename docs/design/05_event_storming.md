# Event Storming
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`, `02_product_requirement_document.md`, `03_software_requirement_specification.md`, `04_domain_model.md`
**Feeds Into:** `06_architecture.md`, `07_sequence_diagrams.md`, `09_api_specification.md`, `14_workflow_engine.md`

---

## 1. Purpose

This document is the output of a (documented, not literally workshopped in real time) **Event Storming exercise** — the standard DDD technique of mapping a system as a timeline of **Domain Events** (orange), triggered by **Commands** (blue), executed against **Aggregates** (yellow), enforced or reacted to by **Policies** (lilac), reading/writing **Read Models** (green), and touching **External Systems** (pink), with **Hotspots** (red) marking open questions or risk areas.

Where `04_domain_model.md` defines the static structure (what things *are*), this document defines the **dynamic process** (what *happens*, in what order, and who reacts to what). It is the bridge between domain modeling and system architecture — `06_architecture.md` uses this timeline directly to justify the choice of event-driven architecture, queue topology, and worker design, and `07_sequence_diagrams.md` expands selected flows from this document into detailed request/response sequences.

## 2. Scope

### 2.1 In Scope
- Full end-to-end process timeline: onboarding → ingestion → extraction → finalization → analytics/budget → notification.
- Commands, Events, Policies, Read Models, External Systems, and Aggregates for each process step.
- Identified hotspots (risk areas, open design questions) requiring explicit resolution in later documents.
- Process-level swimlane view per bounded context.

### 2.2 Out of Scope
- Technology/infrastructure mapping (message broker choice, queue names) → `06_architecture.md`
- Detailed request/response payloads → `09_api_specification.md`, `07_sequence_diagrams.md`
- Workflow orchestration tooling specifics (n8n) → `14_workflow_engine.md`

---

## 3. Legend

| Notation | Meaning |
|---|---|
| 🟦 Command | An intent to change state, issued by a user, a schedule, or another system |
| 🟧 Event | A fact that has happened, named in past tense |
| 🟨 Aggregate | The consistency boundary a command is executed against |
| 🟪 Policy | A reactive rule: "when Event X happens, issue Command Y" |
| 🟩 Read Model | A query-optimized projection consumed by users or other processes |
| 🟥 Hotspot | An open question, risk, or design decision requiring explicit resolution |
| 🩷 External System | A third-party system the process touches |

---

## 4. Architecture: Full Process Timeline (High-Level)

```
[Onboarding] → [Ingestion] → [Extraction/Understanding] → [Confidence Gating]
     → [Finalization] → [Categorization] → [Analytics Projection] → [Budget Evaluation]
     → [Notification Composition] → [Delivery]
```

Each phase below is expanded into its full Command/Event/Policy/Aggregate/External System detail.

---

## 5. Phase 1: Onboarding & Connector Setup

| Element | Detail |
|---|---|
| 🟦 Command | `InitiateGoogleSignIn` |
| 🩷 External System | Google OAuth 2.0 |
| 🟧 Event | `UserSignedIn` |
| 🟨 Aggregate | `User` (Identity & Access) |
| 🟦 Command | `LinkGmailAccount` |
| 🩷 External System | Google OAuth 2.0 (consent + token exchange) |
| 🟧 Event | `GmailAccountLinked` |
| 🟨 Aggregate | `GmailAccount` (Ingestion Context) |
| 🟪 Policy | *When `GmailAccountLinked` → issue `StartInitialFullSync`* |
| 🟪 Policy | *When `GmailAccountLinked` → issue `RegisterGmailWatch`* |
| 🟥 Hotspot | **What happens if the user links the same Gmail address twice (e.g., under two platform accounts)?** Requires a uniqueness policy decision — resolved in `08_database_design.md` (unique constraint on `email_address` across the `gmail_accounts` table, first-linker wins, second attempt surfaces a clear conflict error). |

### 5.1 Swimlane: Identity & Access → Ingestion

```
User            [InitiateGoogleSignIn] ──▶ UserSignedIn
                 [LinkGmailAccount]    ──▶ GmailAccountLinked
                                              │
                                              ▼ (policy)
Ingestion                          StartInitialFullSync ──▶ InitialSyncStarted
                                    RegisterGmailWatch   ──▶ WatchRegistered
```

---

## 6. Phase 2: Ingestion

| Element | Detail |
|---|---|
| 🟦 Command | `StartInitialFullSync` |
| 🩷 External System | Gmail API (`messages.list`) |
| 🟧 Event | `InitialSyncStarted` → (per message) `MessageIngested` → `InitialSyncCompleted` |
| 🟦 Command | `RegisterGmailWatch` |
| 🩷 External System | Gmail API (`users.watch`), Google Cloud Pub/Sub |
| 🟧 Event | `WatchRegistered` |
| 🟨 Aggregate | `GmailAccount` |
| 🩷 External System | Pub/Sub push webhook (inbound) |
| 🟦 Command | `ProcessPubSubNotification` |
| 🟪 Policy | *When Pub/Sub push received → issue `RunIncrementalSync`* |
| 🟦 Command | `RunIncrementalSync` |
| 🩷 External System | Gmail API (`history.list`) |
| 🟧 Event | `MessageIngested` (per new message), or `HistorySyncFallbackTriggered` (if `historyId` expired) |
| 🟨 Aggregate | `IngestedMessage` |
| 🟪 Policy | *When `MessageIngested` → issue `ClassifyAndRouteMessage`* |
| 🟪 Policy | *When Watch nears expiry (scheduled check) → issue `RenewGmailWatch`* |
| 🟧 Event | `WatchRenewed` / `WatchRenewalFailed` |
| 🟥 Hotspot | **Gmail Watch renewal failure combined with a missed Pub/Sub push creates a silent gap.** Mitigation: a scheduled reconciliation job (`RunIncrementalSync` triggered on a fixed interval, e.g., every 15 minutes, independent of push) acts as a safety net — this dual-trigger design (push + poll fallback) must be explicit in `06_architecture.md` and `12_gmail_connector.md`, not left as push-only. |

### 6.1 Swimlane: Ingestion

```
Scheduler/PubSub   [ProcessPubSubNotification] ──▶ (policy) ──▶ RunIncrementalSync
                    [RunIncrementalSync]        ──▶ MessageIngested / HistorySyncFallbackTriggered
                    [RenewGmailWatch] (scheduled)──▶ WatchRenewed / WatchRenewalFailed
```

---

## 7. Phase 3: Understanding (Classification & Extraction)

| Element | Detail |
|---|---|
| 🟦 Command | `ClassifyAndRouteMessage` |
| 🟨 Aggregate | `IngestedMessage` |
| 🟩 Read Model | `known_sender_registry` (lookup) |
| 🟧 Event | `MessageRoutedToTemplateParser` / `MessageRoutedToAIExtractor` / `MessageDiscarded` |
| 🟪 Policy | *When `MessageRoutedToTemplateParser` → issue `ApplySenderTemplate`* |
| 🟦 Command | `ApplySenderTemplate` |
| 🟨 Aggregate | `SenderTemplate` |
| 🟧 Event | `TemplateExtractionSucceeded` / `TemplateExtractionFailed` |
| 🟪 Policy | *When `TemplateExtractionFailed` → issue `InvokeAIExtractor` (fallback)* |
| 🟪 Policy | *When `MessageRoutedToAIExtractor` → issue `InvokeAIExtractor`* |
| 🟦 Command | `InvokeAIExtractor` |
| 🩷 External System | LLM Provider (OpenAI-compatible API) |
| 🟧 Event | `AIExtractionCompleted` / `AIExtractionSchemaInvalid` |
| 🟪 Policy | *When `AIExtractionSchemaInvalid` → issue `RePromptWithStrictSchema` (once), then if still invalid → `RouteToManualReviewQueue`* |
| 🟧 Event | `FinancialEventCandidateCreated` (emitted by both `TemplateExtractionSucceeded` and `AIExtractionCompleted` paths — this is the convergence point where the two extraction paths produce a symmetric output per `04_domain_model.md` Section 7.2) |
| 🟥 Hotspot | **Cost control for AI extraction fallback storms.** If a sender's template breaks and the sender is very high-volume, every message from that sender suddenly floods the AI Extractor. Requires a circuit-breaker/rate-limiting policy at the Understanding Context level, and urgent alerting to Administration (`ParserFailureRateSpiked`) — flagged for explicit design in `06_architecture.md` and `11_ai_blueprint.md`. |

### 7.1 Swimlane: Understanding

```
Ingestion Event    MessageIngested
                        │ (policy)
                        ▼
Understanding       ClassifyAndRouteMessage ──▶ MessageRoutedTo{Template|AI}/Discarded
                        │                              │
                (policy)▼                      (policy)▼
                  ApplySenderTemplate           InvokeAIExtractor
                        │                              │
                TemplateExtractionSucceeded    AIExtractionCompleted
                  /Failed                       /SchemaInvalid
                        │                              │
                        └──────────┬───────────────────┘
                                   ▼
                    FinancialEventCandidateCreated
```

---

## 8. Phase 4: Confidence Gating & Finalization

| Element | Detail |
|---|---|
| 🟪 Policy | *When `FinancialEventCandidateCreated` → issue `EvaluateConfidenceGate`* |
| 🟦 Command | `EvaluateConfidenceGate` |
| 🟨 Aggregate | `FinancialEvent` (being created) |
| 🟧 Event | `FinancialEventFinalized` (confidence ≥ threshold) OR `FinancialEventPendingReview` (confidence < threshold) |
| 🟪 Policy | *When `FinancialEventPendingReview` → issue `ComposeReviewRequestNotification`* |
| 🟦 Command | `ConfirmPendingReviewEvent` (user-initiated, e.g., via WhatsApp reply or app) |
| 🟧 Event | `FinancialEventFinalized` (user-confirmed path) |
| 🟦 Command | `SubmitEventOverride` (user-initiated correction on any event, review or already finalized) |
| 🟧 Event | `FinancialEventOverridden` |
| 🟥 Hotspot | **Confirmation via WhatsApp reply requires parsing free-text/button replies back into a structured command.** This is itself a mini extraction problem (though far simpler — bounded to Y/N/edit intents) and must be explicitly designed in `15_notification_system.md`, not assumed to be "just an API call." |

### 8.1 Swimlane: Financial Core

```
Understanding Event   FinancialEventCandidateCreated
                            │ (policy)
                            ▼
Financial Core        EvaluateConfidenceGate
                            │
              ┌─────────────┴─────────────┐
              ▼                           ▼
      FinancialEventFinalized     FinancialEventPendingReview
                                          │ (policy)
                                          ▼
                                ComposeReviewRequestNotification
                                          │
                              (user)  ConfirmPendingReviewEvent
                                          │
                                  FinancialEventFinalized
```

---

## 9. Phase 5: Categorization

| Element | Detail |
|---|---|
| 🟪 Policy | *When `FinancialEventFinalized` (or `FinancialEventCandidateCreated`, whichever ordering is chosen — see Hotspot below) → issue `AssignCategory`* |
| 🟦 Command | `AssignCategory` |
| 🟩 Read Model | User's historical merchant→category mapping; global merchant→category mapping |
| 🩷 External System | LLM Provider (fallback categorization only) |
| 🟧 Event | `CategoryAssignmentChanged` |
| 🟦 Command | `SubmitCategoryCorrection` (user-initiated) |
| 🟧 Event | `CategoryAssignmentChanged` (source = `manual`), which also feeds back into the user's historical mapping read model |
| 🟥 Hotspot | **Should categorization happen before or after confidence gating?** Two options: (a) categorize only finalized events — simpler, but pending-review events show no category hint to the user reviewing them; (b) categorize all candidates immediately, independent of gating — better UX for the review queue (user sees a suggested category while confirming), but means categorization work is sometimes "wasted" on later-discarded candidates. **Resolution:** Categorize at candidate-creation time (option b), since categorization cost (mostly read-model lookups, only occasionally an LLM call) is low relative to the UX benefit, and discarded/never-confirmed candidates are rare. This is carried into `06_architecture.md` as the defined event ordering. |

---

## 10. Phase 6: Analytics Projection (Read-Model Building)

| Element | Detail |
|---|---|
| 🟪 Policy | *When `FinancialEventFinalized` / `FinancialEventOverridden` / `FinancialEventArchived` / `CategoryAssignmentChanged` → issue `ProjectAnalyticsReadModels`* |
| 🟦 Command | `ProjectAnalyticsReadModels` |
| 🟩 Read Model | `CashFlowView`, `CategoryBreakdown`, `MerchantRanking`, `TrendView` |
| 🟥 Hotspot | **Read-model projection lag.** Since Analytics is CQRS-derived (per `04_domain_model.md` Section 10), there is an inherent (small) delay between an event finalizing and the read-model reflecting it. This must be bounded and monitored (an explicit NFR, already captured as SRS 6.1's analytics latency target applies to *query* time, but *projection freshness* is a distinct metric needing its own target — flagged for `06_architecture.md` and `17_observability.md`). |

---

## 11. Phase 7: Budget Evaluation

| Element | Detail |
|---|---|
| 🟪 Policy | *When `FinancialEventFinalized` (where `event_type = Expense`) → issue `EvaluateBudgetImpact`* |
| 🟦 Command | `EvaluateBudgetImpact` |
| 🟨 Aggregate | `Budget` |
| 🟧 Event | `BudgetThresholdCrossed` (only if a not-yet-fired threshold is newly crossed — idempotent per `04_domain_model.md` INV on `fired_alerts`) |
| 🟪 Policy | *When `BudgetThresholdCrossed` → issue `ComposeBudgetAlertNotification`* |

---

## 12. Phase 8: Notification Composition & Delivery

| Element | Detail |
|---|---|
| 🟦 Command | `ComposeDailySummary` / `ComposeWeeklySummary` / `ComposeMonthlyReport` (scheduler-triggered) |
| 🟦 Command | `ComposeBudgetAlertNotification` / `ComposeReviewRequestNotification` (policy-triggered) |
| 🟩 Read Model | Analytics read models (Section 10) consumed as composition input |
| 🟨 Aggregate | `NotificationJob` |
| 🟧 Event | `NotificationComposed` |
| 🟪 Policy | *When `NotificationComposed` → issue `DeliverViaWhatsApp`* |
| 🟦 Command | `DeliverViaWhatsApp` |
| 🩷 External System | WhatsApp Business API |
| 🟧 Event | `NotificationDelivered` / `NotificationFailed` |
| 🟪 Policy | *When `NotificationFailed` (transient) → issue `RetryDelivery` (bounded backoff)* |
| 🟥 Hotspot | **WhatsApp template pre-approval constraint.** Proactive messages (daily/weekly/monthly summaries, budget alerts) fall outside any user-initiated session window, meaning every distinct message *shape* needs a pre-approved WhatsApp template. Dynamic content (amounts, merchant names) must fit into approved template placeholders — this constrains notification composition flexibility and must be explicitly designed around in `15_notification_system.md`, including a process for requesting new template approvals as notification types evolve. |

### 12.1 Swimlane: Engagement

```
Scheduler/Policy   Compose{Daily|Weekly|Monthly|BudgetAlert|ReviewRequest}
                        │
                  NotificationComposed
                        │ (policy)
                        ▼
                  DeliverViaWhatsApp ──▶ (WhatsApp API) ──▶ NotificationDelivered/Failed
                                                                  │ (policy, if Failed+transient)
                                                                  ▼
                                                            RetryDelivery
```

---

## 13. Phase 9: Account Lifecycle (Disconnection & Data Purge)

| Element | Detail |
|---|---|
| 🟦 Command | `DisconnectGmailAccount` |
| 🟨 Aggregate | `GmailAccount` |
| 🟧 Event | `AccountDisconnected` |
| 🟪 Policy | *When `AccountDisconnected` → issue `StopGmailWatch`* |
| 🟦 Command | `RequestDataPurge` (user choice at disconnect time, per FR-USER-02) |
| 🟧 Event | `UserDataPurgeRequested` |
| 🟪 Policy | *When `UserDataPurgeRequested` → issue `ArchiveFinancialEvents` (for that connector's events)* |
| 🟦 Command | `ArchiveFinancialEvents` |
| 🟨 Aggregate | `FinancialEvent` (batch operation) |
| 🟧 Event | `FinancialEventArchived` (per event) |
| 🟪 Policy | *When retention window elapses post-archival → issue `PermanentlyEraseFinancialEvent`* (scheduled sweep) |
| 🟥 Hotspot | **Cross-context cascading deletion correctness.** A purge must correctly cascade: `FinancialEvent` archival → Analytics read-model exclusion → raw content erasure → audit log retention (audit entries about the deletion itself must NOT be deleted, per typical compliance practice) — this cross-cutting cascade must be explicitly enumerated and tested (`18_testing_strategy.md`), since a partial cascade is a GDPR compliance risk. |

---

## 14. Consolidated Hotspot Register

| # | Hotspot | Resolution Owner (Document) |
|---|---|---|
| H1 | Duplicate Gmail account linking across platform users | `08_database_design.md` |
| H2 | Silent sync gap from missed Watch renewal + missed push | `06_architecture.md`, `12_gmail_connector.md` |
| H3 | AI extraction fallback storm on template breakage (cost/rate-limit risk) | `06_architecture.md`, `11_ai_blueprint.md` |
| H4 | WhatsApp reply parsing for pending-review confirmation | `15_notification_system.md` |
| H5 | Categorization ordering relative to confidence gating | Resolved in this document (Section 9) — carried forward to `06_architecture.md` |
| H6 | Analytics read-model projection lag as a distinct NFR from query latency | `06_architecture.md`, `17_observability.md` |
| H7 | WhatsApp pre-approved template constraint on dynamic notification content | `15_notification_system.md` |
| H8 | Cross-context cascading deletion correctness on data purge | `18_testing_strategy.md`, `16_security.md` |

These hotspots are **mandatory checklist items** for the documents listed — each owning document must explicitly state how the hotspot is resolved, not silently proceed past it.

---

## 15. Detailed Explanation: Why Policies Are Modeled Explicitly (Not Left Implicit)

A common event-storming shortcut is to skip explicitly naming policies and instead let "the system reacts somehow." This document deliberately names every policy (🟪) because:

1. **Each named policy is a candidate consumer/handler boundary** in the eventual worker architecture (`10_backend_blueprint.md`) — an explicit policy list becomes the direct blueprint for how many distinct background job types the system needs.
2. **Idempotency and retry design require knowing exactly which policy re-fires on redelivery.** A policy like *"When `FinancialEventFinalized` → issue `EvaluateBudgetImpact`"* must be safe to execute twice (already covered by the `fired_alerts` idempotency in `04_domain_model.md`) — this is only verifiable if the policy is named and isolated, not buried in a monolithic "on event, do everything" handler.
3. **Policies are the natural seam for the Outbox pattern** (`06_architecture.md`): each policy corresponds to an outbox-relayed message, giving a clean mapping from domain design to the transactional messaging mechanism.

---

## 16. Component Responsibilities (Process-Level)

| Process Phase | Responsibility |
|---|---|
| Onboarding | Establish identity and connector linkage; trigger initial sync and watch registration |
| Ingestion | Reliably capture raw signals exactly once (at the effect level) from Gmail |
| Understanding | Convert raw signals into a symmetric, confidence-scored candidate event, deterministically where possible |
| Confidence Gating & Finalization | Guard the ledger's integrity; provide a user-facing correction path |
| Categorization | Enrich finalized (and pending) events with actionable, learnable categorization |
| Analytics Projection | Maintain fresh, correct, rebuildable read models |
| Budget Evaluation | Detect and idempotently alert on threshold crossings |
| Notification | Compose and reliably deliver user-facing communication |
| Account Lifecycle | Correctly and completely cascade disconnection/purge across all contexts |

---

## 17. Trade-offs

| Decision | Trade-off |
|---|---|
| Dual-trigger ingestion (push + scheduled poll fallback) | Slightly higher baseline load (periodic polling even when push is healthy) in exchange for closing the "silent gap" hotspot (H2) definitively rather than hoping push reliability is sufficient. |
| Categorize-before-gating ordering | Some categorization work is performed on candidates that are ultimately discarded/never confirmed, in exchange for a materially better review-queue UX. |
| Explicit policy-per-reaction modeling | More named components to design and test individually, in exchange for a direct, unambiguous mapping to the eventual worker/queue architecture. |

## 18. Advantages
- The full event timeline gives every engineering discipline (backend, AI, DevOps, QA) a shared, unambiguous process reference before any technology decisions are made.
- Hotspots are captured and assigned an owning document now, preventing them from being silently discovered (and expensively fixed) during implementation.
- The Policy-as-explicit-component approach directly de-risks the transition to the Outbox/Inbox event-driven architecture in `06_architecture.md`.

## 19. Disadvantages
- The level of detail here (naming every command/event/policy) is heavier than a typical lightweight event-storming session — an accepted cost given the "never simplify the design" directive, but it does mean this document requires active maintenance as the process evolves.
- Some hotspots (e.g., H3, the AI fallback storm) don't have a resolution *in this document* — they are deliberately deferred to the correct owning document, which means this document alone is not sufficient for a reader to understand the full resolved design; it must be read alongside its downstream dependents.

## 20. Future Improvements
- Extend the timeline with a "Conversational Query" phase once Phase 3 (Advisory Intelligence, per `01_product_vision.md`) is prioritized — this will introduce new commands (`SubmitFinancialQuery`) and events (`FinancialQueryAnswered`) not yet modeled here.
- Add an explicit "Anomaly Detection" phase (fraud-pattern signals, duplicate charge detection across sources) once multi-connector ingestion is active.
- Revisit H5's categorize-before-gating decision once real usage data shows the actual "wasted categorization" rate on discarded candidates — if it's negligible, no change; if significant (e.g., due to a very noisy unknown-sender long tail), reconsider deferring categorization to post-finalization only.

## 21. Best Practices
- Any new feature must be traced through this event-storming format (Command → Aggregate → Event → Policy → Read Model) before implementation begins, to ensure new reactive chains are fully considered (including idempotency and failure paths) rather than bolted on ad hoc.
- Hotspots must never be silently resolved inside code comments — resolutions belong in the owning document referenced in Section 14, keeping the documentation set as the single source of truth.

## 22. Production Considerations
- The dual-trigger ingestion fallback (H2) must have its polling interval tuned against Gmail API quota costs — too frequent a poll wastes quota; too infrequent widens the silent-gap window. This tuning must be revisited using production quota-usage data (see `23_scalability.md`).
- The AI fallback storm circuit breaker (H3) must have a defined production runbook: what does an on-call engineer do when it trips? (e.g., pause ingestion for the affected sender, prioritize a template fix) — this belongs in `19_deployment.md` operational runbooks.
- Analytics projection lag (H6) must be exposed as a first-class production metric/dashboard, not an implicit assumption that "the read model is always fresh."

---

**Next Document:** `06_architecture.md` — translates this process timeline and the bounded context map from `04_domain_model.md` into a concrete technical architecture: service boundaries, the Outbox/Inbox event-driven mechanism, worker topology, and technology-to-component mapping using the specified Go/PostgreSQL/Redis/Asana stack.
