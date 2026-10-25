# Sprint 1 — Identity & Access + Database Foundations

**Theme:** Stand up the `identity` module end-to-end (the simplest full module, good as a template for the rest) and scaffold every other module's database schema up front so later sprints aren't blocked on migration work.

**Primary documents:** `08_database_design.md` (all), `16_security.md` §4–6, `09_api_specification.md` §5, `10_backend_blueprint.md` §3–12

**Entry criteria:** Sprint 0 complete.

---

## Checklist

### Database — All Schemas (Scaffolding Only This Sprint)
- [x] Create all 8 PostgreSQL schemas: `identity`, `ingestion`, `understanding`, `financialcore`, `budget`, `analytics`, `engagement`, `administration` (`08_database_design.md` §3)
- [x] Per-module database roles provisioned, scoped to their own schema only **[GATE]** (`06_architecture.md` §5.3, `08_database_design.md` §3)
- [x] `db/migrations/<schema>/` directories created for each schema
- [x] Goose migration: `identity.users`, `identity.oauth_tokens` (`08_database_design.md` §6)
- [x] Goose migration: shared `outbox_events` table shape, instantiated in `identity` schema first as the reference implementation (`08_database_design.md` §4)
- [x] Goose migration: `administration.audit_log`, with explicit `DELETE`/`UPDATE` grants **withheld** from `administration_svc` role **[GATE]** (`08_database_design.md` §13.1, `16_security.md` §8.2)

### Identity Module — Domain Layer
- [x] `internal/identity/domain/user.go` — `User` aggregate
- [x] `internal/identity/domain/oauth_token.go` — `OAuthToken` entity
- [x] `internal/identity/domain/repository.go` — repository interfaces
- [x] `internal/identity/domain/errors.go` — sentinel errors

### Identity Module — Application Layer
- [x] Command: `LinkGoogleAccount`
- [x] Command: `RefreshOAuthToken`
- [x] Command: `UpdateUserSettings`
- [x] Query: `GetUserByID`

### Identity Module — Adapters
- [x] `adapter/external/googleoauthclient.go` — OAuth 2.0 Authorization Code + PKCE flow (`16_security.md` §5.1)
  - [x] Scopes limited to `gmail.readonly`, `openid`, `email`, `profile` **[GATE — never request write scopes]**
  - [x] `access_type=offline`, `prompt=consent`, `include_granted_scopes=true` set correctly (`12_gmail_connector.md` §4.2)
- [x] `adapter/postgres/repository.go` + sqlc queries for `users`, `oauth_tokens`
- [x] `adapter/http/handler.go`: `POST /auth/google/initiate`, `GET /auth/google/callback` (`09_api_specification.md` §5.1)
- [x] `module.go` composition

### Encryption & Session Security
- [x] KMS integration stood up; envelope encryption implemented for `oauth_tokens` **[GATE]** (`16_security.md` §4.3)
- [x] Platform session JWT issuance (short-lived, KMS-managed signing key) (`16_security.md` §5.1)
- [x] Opaque, DB-backed, revocable refresh token implementation (`16_security.md` §5.1)
- [x] OAuth `state` CSRF parameter: random, single-use, short-TTL (`16_security.md` §5.2)

### RBAC Scaffolding
- [x] `user` role claim model implemented
- [x] `ops` role claim model implemented (no endpoints use it yet — scaffolding only)
- [x] Row-level ownership-check convention established: every user-scoped repository method signature requires `userID` as a mandatory parameter **[GATE]** (`16_security.md` §6.2, `20_coding_convention.md` §9.2)

### Database Schemas for Other Modules (Scaffolding Only)
- [x] Goose migrations: `ingestion.gmail_accounts`, `ingestion.ingested_messages`, `ingestion.sender_watch_log` (tables only, not yet consumed by code)
- [x] Goose migrations: `understanding.sender_templates`, `understanding.extraction_results`
- [x] Goose migrations: `financialcore.financial_events`, `event_overrides`, `category_assignments`, `inbox_processed` **[GATE — get this right now, Sprint 6 depends on it]**
- [x] Goose migrations: `budget.budgets`, `budget_fired_alerts`, `savings_goals`
- [x] Goose migrations: `analytics.*` views, `projection_checkpoint`
- [x] Goose migrations: `engagement.notification_jobs`, `notification_preferences`
- [x] Every table reviewed against `08_database_design.md`'s constraint list — no cross-schema FK anywhere **[GATE]**

### Testing
- [x] Unit tests: `User`/`OAuthToken` domain invariants
- [x] Integration test: full OAuth link flow against a mocked Google OAuth endpoint
- [x] Integration test: duplicate-email-linking conflict returns `409`, no duplicate row (H1, per `07_sequence_diagrams.md` Diagram 1)

### Frontend (Parallel Track)
- [x] Frontend project scaffolded (React + Vite), consuming `09_api_specification.md` exclusively — no direct DB/API-provider access **[GATE]**
- [x] API client wrapper with `Authorization: Bearer` session-token handling, decimal-string money parsing (never `parseFloat` on money fields — match the backend's decimal-string convention)
- [x] Landing/login page: "Sign in with Google" → `POST /auth/google/initiate` → redirect
- [x] OAuth callback page: handles `GET /auth/google/callback` response, stores session token, redirects to app shell
- [x] Session refresh handling (silent refresh before access-token expiry, redirect to login on refresh failure)
- [x] Basic app shell/layout (nav, empty dashboard placeholder) — nothing functional yet beyond auth
- [x] Error boundary + a consistent API-error-envelope display component (matches `09_api_specification.md` §7.2's shape) — build this once, reuse every sprint

### Observability
- [x] Structured logging (`slog`) wired at the composition root per `06_architecture.md` §10 — `identity` module logs include `requestId`/`traceId`
- [x] OpenTelemetry tracer initialized; root span created on every HTTP request
- [x] Frontend: basic error/exception reporting wired (e.g., Sentry or equivalent) — not optional, catch auth-flow failures from day one

---

## Exit Criteria

- [x] A user can complete Google OAuth sign-in end to end locally and receive a platform session token
- [x] Tokens are verifiably encrypted at rest (inspect the DB directly — ciphertext, not plaintext)
- [x] All 8 schemas exist with their full MVP table set, reviewed against `08_database_design.md`
- [x] `identity` module passes `make lint`, `make test-unit`, `make test-integration` cleanly
- [x] Module-boundary linter confirms `identity` has zero forbidden cross-schema/cross-module access
- [x] Frontend: user can click "Sign in with Google" in a browser, complete consent, and land on an authenticated empty app shell

