# API Specification
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `06_architecture.md`, `07_sequence_diagrams.md`, `08_database_design.md`
**Feeds Into:** `10_backend_blueprint.md`, `12_gmail_connector.md`, `15_notification_system.md`, `18_testing_strategy.md`

**Naming Convention Note:** Per project convention, all Go application-layer identifiers use **camelCase**; this carries into the JSON wire format of this API — every request/response field is **camelCase** (e.g., `financialEventId`, `occurredAt`). Database columns remain snake_case per `08_database_design.md`; the mapping between the two happens in each module's adapter layer (`10_backend_blueprint.md`).

---

## 1. Purpose

This document specifies the platform's public HTTP API: the contract that first-party clients (future web/mobile apps) and, where applicable, administrative tooling use to interact with the system. It is the authoritative source for endpoint paths, request/response DTOs, authentication, error semantics, and pagination — and is the document from which the actual OpenAPI 3.1 YAML/JSON artifact (an implementation deliverable) is mechanically derived.

Every endpoint here traces back to a functional requirement (`FR-*`/`SRS-FR-*`) and a domain command/query from `04_domain_model.md` and `05_event_storming.md`. No endpoint may be added during implementation without a corresponding entry in this document.

## 2. Scope

### 2.1 In Scope
- REST API design principles and conventions for this platform.
- Authentication and session model.
- Full endpoint catalog across all modules, with request/response DTO shapes.
- Standard error response format and status code conventions.
- Pagination, filtering, and sorting conventions.
- API versioning strategy.
- Webhook endpoints (inbound, for Gmail Pub/Sub and WhatsApp delivery status).

### 2.2 Out of Scope
- Internal Asynq task payload shapes (those are not part of the public API surface) → `10_backend_blueprint.md`.
- Gmail/WhatsApp/LLM provider API contracts themselves (we are a consumer of those, not a definer) → `12_gmail_connector.md`, `15_notification_system.md`, `11_ai_blueprint.md`.
- Rate-limiting infrastructure detail → `16_security.md`, `17_observability.md`.

---

## 3. Architecture: API Surface Overview

```
/api/v1
│
├── /auth
│   ├── POST   /auth/google/initiate
│   └── GET    /auth/google/callback
│
├── /me
│   ├── GET    /me
│   ├── PATCH  /me/settings
│   ├── GET    /me/gmail-accounts
│   ├── POST   /me/gmail-accounts               (link additional account)
│   ├── DELETE /me/gmail-accounts/{id}           (disconnect, ?purge=true|false)
│
├── /financial-events
│   ├── GET    /financial-events
│   ├── GET    /financial-events/{id}
│   ├── POST   /financial-events                (manual entry — Could Have, Phase-gated)
│   ├── PATCH  /financial-events/{id}            (submit override)
│   ├── POST   /financial-events/{id}/confirm    (confirm pending-review)
│   └── GET    /financial-events/{id}/overrides
│
├── /analytics
│   ├── GET    /analytics/cash-flow
│   ├── GET    /analytics/category-breakdown
│   ├── GET    /analytics/merchant-ranking
│   └── GET    /analytics/trend
│
├── /budgets
│   ├── GET    /budgets
│   ├── POST   /budgets
│   ├── PATCH  /budgets/{id}
│   ├── DELETE /budgets/{id}
│   ├── GET    /savings-goals
│   ├── POST   /savings-goals
│   └── PATCH  /savings-goals/{id}
│
├── /notifications
│   ├── GET    /notifications/preferences
│   └── PATCH  /notifications/preferences
│
├── /admin  (internal/ops-authenticated only — see Section 6.3)
│   ├── GET    /admin/connectors/health
│   ├── GET    /admin/parsers/metrics
│   ├── GET    /admin/prompts/metrics
│   └── GET    /admin/audit-log
│
└── /webhooks (unauthenticated by user-session; secured by provider signature — Section 6.4)
    ├── POST   /webhooks/gmail/pubsub
    └── POST   /webhooks/whatsapp/delivery-status
```

---

## 4. API Design Principles

1. **Resource-oriented REST**, not RPC-style action endpoints, except where the domain action is genuinely not a CRUD operation on a resource (e.g., `/financial-events/{id}/confirm` is a domain command, modeled as a sub-resource action per common REST convention for such cases).
2. **Every response envelope is consistent** (Section 7) regardless of endpoint, to simplify client-side handling.
3. **Read endpoints serve exclusively from Analytics read models** where applicable (per the CQRS boundary in `06_architecture.md` Section 7) — `/analytics/*` endpoints never query `financialcore` directly.
4. **Write endpoints return `202 Accepted`** where the underlying effect is asynchronous (per `07_sequence_diagrams.md`'s async-response pattern for onboarding/purge), and `200`/`201` where the effect is synchronous and immediately consistent (e.g., a budget definition write).
5. **Idempotency-Key header support** on all state-changing `POST` endpoints, allowing clients to safely retry a request without risk of duplicate effect — implemented via the same Inbox-pattern infrastructure from `06_architecture.md` Section 6.2, keyed on the client-supplied `Idempotency-Key`.

---

## 5. Authentication & Session Model

### 5.1 Google OAuth Flow (traces to SRS-FR-AUTH-01)

**`POST /api/v1/auth/google/initiate`**
- **Response `200`:**
```
{
  "authorizationUrl": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "opaque-csrf-state-token"
}
```

**`GET /api/v1/auth/google/callback?code=...&state=...`**
- **Processing:** Exchanges code for tokens (server-side), creates/matches `User`, issues a platform session.
- **Response `200`:**
```
{
  "sessionToken": "eyJhbGciOi...",
  "expiresAt": "2026-08-12T00:00:00Z",
  "user": {
    "id": "b3f1...",
    "email": "user@example.com",
    "displayName": "Jane Doe"
  }
}
```
- **Error `400`:** invalid/expired `code` or `state` mismatch (possible CSRF) → standard error envelope (Section 7.2), `errorCode: "OAUTH_CALLBACK_INVALID"`.

### 5.2 Session Token Usage

All authenticated endpoints require `Authorization: Bearer <sessionToken>`. Session tokens are short-lived JWTs (platform-issued, distinct from Google's own OAuth tokens, which never leave the server per least-privilege handling in `16_security.md`); refresh mechanics are specified in `16_security.md`.

### 5.3 Admin Endpoints

`/admin/*` endpoints require a session token additionally carrying an `role: "ops"` claim, provisioned only for internal operations staff — never a self-service platform-user role. Full RBAC design is in `16_security.md`.

### 5.4 Webhook Authentication

- `/webhooks/gmail/pubsub`: Validates Google-signed JWT in the `Authorization` header per Google's Pub/Sub push authentication scheme (per `07_sequence_diagrams.md` Diagram 2's explicit pre-enqueue verification step).
- `/webhooks/whatsapp/delivery-status`: Validates the WhatsApp Business API's request signature (`X-Hub-Signature-256`) against the configured app secret.

---

## 6. Endpoint Catalog

### 6.1 User & Account Management

**`GET /me`** *(traces to FR-USER-01)*
- **Response `200`:**
```
{
  "id": "b3f1...",
  "email": "user@example.com",
  "displayName": "Jane Doe",
  "timezone": "Asia/Jakarta",
  "gmailAccounts": [
    { "id": "a1c2...", "emailAddress": "jane@gmail.com", "status": "active" }
  ]
}
```

**`PATCH /me/settings`**
- **Request:**
```
{
  "timezone": "Asia/Jakarta",
  "notificationDailyTime": "07:30"
}
```
- **Response `200`:** updated `User` representation.

**`GET /me/gmail-accounts`** — lists linked accounts with sync status (`linking`, `initial_sync`, `active`, `disconnected`, `error`).

**`POST /me/gmail-accounts`** *(traces to FR-USER-01)*
- **Response `202 Accepted`:**
```
{
  "authorizationUrl": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "opaque-csrf-state-token"
}
```
- Follows the same OAuth callback flow as Section 5.1, associating the resulting account with the already-authenticated user rather than creating a new one.

**`DELETE /me/gmail-accounts/{id}?purge={true|false}`** *(traces to FR-USER-02)*
- **Response `202 Accepted`:**
```
{ "status": "disconnect_initiated", "purgeRequested": true }
```
- Maps directly to `07_sequence_diagrams.md` Diagram 8 — this is the trigger for `DisconnectGmailAccount` + optional `RequestDataPurge`.

### 6.2 Financial Events (Ledger)

**`GET /financial-events`** *(traces to FR-LEDGER-01)*
- **Query params:** `from`, `to` (ISO 8601 dates), `status` (`finalized`\|`pending_review`\|`archived`\|`disputed`), `eventType`, `category`, `merchant`, `cursor`, `limit` (default 50, max 200).
- **Response `200`:**
```
{
  "items": [
    {
      "id": "e7a9...",
      "eventType": "Expense",
      "amount": { "value": "45.00", "currency": "USD" },
      "occurredAt": "2026-08-03T14:22:00Z",
      "merchant": { "rawName": "AMZN Mktp US", "normalizedName": "Amazon" },
      "paymentMethod": { "type": "card", "hint": "**** 4821" },
      "status": "finalized",
      "confidence": 0.94,
      "category": { "value": "Shopping", "source": "ai_inference", "confidence": 0.81 },
      "provenance": {
        "connectorType": "gmail",
        "extractionMethod": "template",
        "templateVersion": "amazon-us-v3"
      }
    }
  ],
  "pagination": { "nextCursor": "eyJvZmZzZXQiOjUwfQ==", "hasMore": true }
}
```

**`GET /financial-events/{id}`** — full detail, including `typeSpecificPayload` (shape varies per `eventType`, per `04_domain_model.md` Section 5.1.2).

**`POST /financial-events`** *(traces to FR-LEDGER-03, Could Have)*
- Manual entry fallback; `provenance.extractionMethod` is forced to `manual` server-side regardless of request body.

**`PATCH /financial-events/{id}`** *(traces to FR-LEDGER-02, SRS-FR-LEDGER-02)*
- **Request:**
```
{ "fields": { "amount": { "value": "42.50", "currency": "USD" }, "category": "Groceries" } }
```
- **Processing:** Creates `EventOverride` record(s) per changed field; does not mutate `provenance`.
- **Response `200`:** updated effective `FinancialEvent` view.

**`POST /financial-events/{id}/confirm`** *(traces to SRS-FR-AI-03's `ConfirmPendingReviewEvent`)*
- **Request:** `{ "confirmed": true }` or `{ "confirmed": false, "corrections": { ... } }`
- **Response `200`:** `{ "status": "finalized" }` or, on rejection, the item remains `pending_review` with corrections applied as overrides.

**`GET /financial-events/{id}/overrides`** — full audit trail of `EventOverride` records for the event (transparency/debug view).

### 6.3 Analytics (Read-Only, CQRS Read-Side)

**`GET /analytics/cash-flow`** *(traces to FR-ANALYTICS-01)*
- **Query params:** `from`, `to`, `granularity` (`daily`\|`monthly`), `currency` (optional filter; omitting returns all currencies present, per `08_database_design.md` Section 14's currency-partitioned strategy).
- **Response `200`:**
```
{
  "buckets": [
    { "periodBucket": "2026-07-01", "granularity": "monthly", "currency": "USD",
      "totalIncome": "5200.00", "totalExpense": "3180.45" }
  ],
  "projectionFreshness": "2026-08-05T09:14:00Z"
}
```
- `projectionFreshness` surfaces the read-model's last-updated timestamp directly to the client, giving transparency into the eventually-consistent nature of the CQRS read side (ties to `analytics_projection_lag_seconds`, `06_architecture.md` Section 6.7).

**`GET /analytics/category-breakdown`** *(FR-ANALYTICS-02)* — analogous shape, grouped by `category`, includes `percentageShare`.

**`GET /analytics/merchant-ranking`** *(FR-ANALYTICS-03)* — grouped by `merchantNormalizedName`, includes `transactionCount`, sorted descending by `totalValue`.

**`GET /analytics/trend`** *(FR-ANALYTICS-04)* — time series of `netFlow` per bucket.

### 6.4 Budget

**`GET /budgets`** / **`POST /budgets`** / **`PATCH /budgets/{id}`** / **`DELETE /budgets/{id}`** *(FR-BUDGET-01)*
- **`POST` Request:**
```
{
  "scope": "category:Dining",
  "periodType": "monthly",
  "thresholdAmount": { "value": "500.00", "currency": "USD" },
  "alertThresholds": [0.8, 1.0]
}
```

**`GET /savings-goals`** / **`POST /savings-goals`** / **`PATCH /savings-goals/{id}`** *(FR-BUDGET-03, Should Have)*
- Includes computed `progressPercentage` derived server-side from net cash flow, per `04_domain_model.md` Section 9.2.

### 6.5 Notifications

**`GET /notifications/preferences`** / **`PATCH /notifications/preferences`** *(FR-NOTIF-05)*
```
{
  "dailySummaryEnabled": true,
  "weeklySummaryEnabled": true,
  "monthlyReportEnabled": true,
  "budgetAlertsEnabled": true,
  "aiRecommendationsEnabled": false
}
```

### 6.6 Administration (Internal, `role: "ops"` Required)

**`GET /admin/connectors/health`** *(FR-ADMIN-01)* — per-account sync lag, watch expiration status, error rates.

**`GET /admin/parsers/metrics`** *(FR-ADMIN-02)* — per-`SenderTemplate` success/failure rate, with a flag on templates whose failure rate has spiked recently (surfacing the H3 signal from `05_event_storming.md`/`06_architecture.md` at the API layer for the ops dashboard).

**`GET /admin/prompts/metrics`** *(FR-ADMIN-03)* — per-`prompt_version` confidence distribution and volume.

**`GET /admin/audit-log`** *(FR-ADMIN-04)* — filterable by `userId`, `action`, date range; read-only, reflecting the append-only, deletion-exempt `audit_log` table from `08_database_design.md` Section 13.1.

### 6.7 Webhooks (Inbound, Provider-Authenticated)

**`POST /webhooks/gmail/pubsub`** — accepts Google Pub/Sub push payload, verifies signature, enqueues `RunIncrementalSync` per `07_sequence_diagrams.md` Diagram 2. Always responds `200` quickly (per Pub/Sub's own retry/ack semantics) regardless of downstream processing outcome, since the actual work is deferred to the async queue.

**`POST /webhooks/whatsapp/delivery-status`** — accepts WhatsApp delivery status callbacks, updates `notification_jobs.delivery_confirmed_at` per `07_sequence_diagrams.md` Diagram 7.

---

## 7. Standard Conventions

### 7.1 Pagination

Cursor-based (not offset-based) pagination is used for all list endpoints, to remain stable under concurrent writes to a continuously-growing ledger — an offset-based scheme would risk skipped/duplicated items as new `FinancialEvent` records are inserted between page requests.

```
{ "pagination": { "nextCursor": "<opaque base64>", "hasMore": true } }
```

### 7.2 Error Envelope

Every non-2xx response uses a consistent shape:

```
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "amount.value must be a positive decimal string",
    "field": "amount.value",
    "requestId": "req_9f2a..."
  }
}
```

| HTTP Status | Usage |
|---|---|
| `400` | Malformed request / validation failure |
| `401` | Missing/invalid session token |
| `403` | Valid session, insufficient permission (e.g., non-ops calling `/admin/*`) |
| `404` | Resource not found or not owned by the requesting user |
| `409` | Conflict (e.g., duplicate Gmail account linking — H1) |
| `422` | Semantically invalid (e.g., overriding `amount.currency` to a different currency, per INV-2's restriction) |
| `429` | Rate-limited |
| `500` | Unexpected server error |
| `503` | Downstream dependency unavailable (e.g., attempting a manual re-extraction while the LLM provider is down) |

`requestId` correlates directly to the OpenTelemetry trace ID (per `06_architecture.md` Section 10), making every error report immediately actionable in Grafana/trace lookup.

### 7.3 Filtering & Sorting

List endpoints support consistent `from`/`to` date-range filtering and a `sort` query parameter (default sort is always the most temporally relevant field descending — `occurredAt` for events, `periodBucket` for analytics).

### 7.4 API Versioning

- URL-path versioning (`/api/v1/...`). A breaking change to any endpoint's contract requires a `/api/v2/` introduction, never an in-place breaking change to `/v1`.
- Additive, backward-compatible changes (new optional fields, new endpoints) do not require a version bump, per standard REST evolution practice.
- Deprecation policy (sunset headers, minimum notice period) is specified in `16_security.md`/`19_deployment.md`'s operational policies.

---

## 8. Detailed Explanation: Why `/financial-events/{id}/confirm` Is Not Just a `PATCH`

A pure-REST purist might model "confirm a pending-review event" as `PATCH /financial-events/{id}` with `{ "status": "finalized" }`. This API deliberately uses a **distinct action endpoint** instead, because:

1. Confirmation is a **domain command** (`ConfirmPendingReviewEvent`, per `05_event_storming.md`) with its own validation rules (only valid from `pending_review` status, triggers a distinct Outbox event `FinancialEventFinalized` with a `user-confirmed` provenance detail) — collapsing it into a generic `PATCH` would blur the line between "user is correcting a field" (which creates an `EventOverride`) and "user is confirming the event is correct as-is" (which does not).
2. This distinction directly matches the two different downstream Policy reactions shown in `07_sequence_diagrams.md` Diagram 4 — keeping the API shape aligned with the domain model avoids an impedance mismatch that would otherwise leak into the backend blueprint's command-handler design.

---

## 9. Component Responsibilities (API-Level)

| Component | Responsibility |
|---|---|
| chi router | Route matching, middleware chain (auth, request-ID injection, rate-limiting, validation) |
| Per-module HTTP handlers | Translate HTTP request → module command/query, translate domain result → DTO |
| `go-playground/validator` | Request DTO structural/format validation before reaching application-layer command handlers |
| OpenAPI spec (generated/maintained artifact) | Machine-readable contract, used for client SDK generation and contract testing (`18_testing_strategy.md`) |

---

## 10. Trade-offs

| Decision | Trade-off |
|---|---|
| Cursor-based pagination everywhere | Slightly more complex client implementation (opaque cursors vs. simple page numbers), in exchange for correctness under concurrent writes to a live ledger. |
| Currency-partitioned analytics responses (no default blending) | Client must handle multi-currency response arrays even for typical single-currency users (a minor, near-zero-cost complexity for the common case), in exchange for never misrepresenting money. |
| Distinct `/confirm` action endpoint vs. generic `PATCH` | One more endpoint to document/test, in exchange for precise alignment with the domain's command vocabulary and cleaner downstream event semantics. |
| Synchronous vs. `202 Accepted` async responses, decided per-endpoint | Requires client-side polling or a status-check pattern for async operations (documented per-endpoint), in exchange for API responsiveness even when underlying work touches slow external systems (Gmail, WhatsApp). |

## 11. Advantages
- Every endpoint is directly traceable to a `FR-*`/`SRS-FR-*` requirement and a domain command/event, leaving no ambiguity about why an endpoint exists or what it must guarantee.
- The consistent error envelope and `requestId`-to-trace correlation make production debugging fast and standardized across the entire API surface.
- Read-only Analytics endpoints being strictly CQRS-read-side keeps the API layer honest about the architecture's consistency model (surfaced explicitly via `projectionFreshness`, not hidden).

## 12. Disadvantages
- Cursor-based pagination is less immediately intuitive for simple client integrations than offset/page-number pagination — a documented, necessary trade-off given the domain's write-heavy, continuously-growing nature.
- The `/admin/*` surface being part of the same API versioning scheme as the public surface means admin tooling changes are technically bound by the same backward-compatibility discipline — acceptable, but worth revisiting if admin tooling needs faster iteration than public API stability allows (noted as a Future Improvement).

## 13. Future Improvements
- Consider splitting `/admin/*` into a fully separate internal API service/versioning track once admin tooling requirements diverge significantly from public API stability needs.
- Introduce a GraphQL or BFF (Backend-for-Frontend) layer if/when multiple first-party client types (web, mobile) develop meaningfully different data-shaping needs that a single REST contract struggles to serve efficiently.
- Add a `/financial-events/query` conversational-style endpoint once Phase 3's conversational AI assistant (per `01_product_vision.md`) is prioritized, likely proxying to a dedicated AI-assistant service rather than the standard CRUD surface.

## 14. Best Practices
- No new endpoint may be implemented without first being added to this document and cross-referenced to a requirement ID — enforced as a PR review checklist item.
- All monetary fields in requests/responses are transmitted as **decimal strings** (never JSON floating-point numbers), matching the `numeric(18,4)` database convention from `08_database_design.md`, to avoid floating-point precision loss across the wire.
- `Idempotency-Key` support must be implemented uniformly using the shared Inbox-pattern infrastructure (`06_architecture.md` Section 6.2), not reimplemented ad hoc per endpoint.

## 15. Production Considerations
- Rate-limiting thresholds per endpoint (especially `/auth/*` and `/webhooks/*`) must be defined and enforced at the Traefik edge layer before launch, detailed in `16_security.md`.
- The OpenAPI artifact generated from this document must be part of the GitLab CI pipeline's contract-testing stage (`18_testing_strategy.md`), failing the build if actual handler behavior drifts from the documented contract.
- `projectionFreshness` and other transparency fields must be actively monitored in production — if freshness regularly exceeds the target from `06_architecture.md` Section 6.7, this is a production incident, not just an API quirk to document around.

---

**Next Document:** `10_backend_blueprint.md` — specifies the Go module implementation blueprint: package layout within each module, command/query handler patterns, sqlc query organization, and the concrete mapping from this API specification and the database design into idiomatic, Clean-Architecture-compliant Go code structure.
