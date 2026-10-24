# Sprint 1 — Identity & Access + Database Foundations

**Theme:** Stand up the `identity` module end-to-end (the simplest full module, good as a template for the rest) and scaffold every other module's database schema up front so later sprints aren't blocked on migration work.

**Primary documents:** `08_database_design.md` (all), `16_security.md` §4–6, `09_api_specification.md` §5, `10_backend_blueprint.md` §3–12

**Entry criteria:** Sprint 0 complete.

---

## Checklist

### Database — All Schemas (Scaffolding Only This Sprint)
- [ ] Create all 8 PostgreSQL schemas: `identity`, `ingestion`, `understanding`, `financialcore`, `budget`, `analytics`, `engagement`, `administration` (`08_database_design.md` §3)
- [ ] Per-module database roles provisioned, scoped to their own schema only **[GATE]** (`06_architecture.md` §5.3, `08_database_design.md` §3)
- [ ] `db/migrations/<schema>/` directories created for each schema
- [ ] Goose migration: `identity.users`, `identity.oauth_tokens` (`08_database_design.md` §6)
- [ ] Goose migration: shared `outbox_events` table shape, instantiated in `identity` schema first as the reference implementation (`08_database_design.md` §4)
- [ ] Goose migration: `administration.audit_log`, with explicit `DELETE`/`UPDATE` grants **withheld** from `administration_svc` role **[GATE]** (`08_database_design.md` §13.1, `16_security.md` §8.2)

### Identity Module — Domain Layer
- [ ] `internal/identity/domain/user.go` — `User` aggregate
- [ ] `internal/identity/domain/oauth_token.go` — `OAuthToken` entity
- [ ] `internal/identity/domain/repository.go` — repository interfaces
- [ ] `internal/identity/domain/errors.go` — sentinel errors

### Identity Module — Application Layer
- [ ] Command: `LinkGoogleAccount`
- [ ] Command: `RefreshOAuthToken`
- [ ] Command: `UpdateUserSettings`
- [ ] Query: `GetUserByID`

### Identity Module — Adapters
- [ ] `adapter/external/googleoauthclient.go` — OAuth 2.0 Authorization Code + PKCE flow (`16_security.md` §5.1)
  - [ ] Scopes limited to `gmail.readonly`, `openid`, `email`, `profile` **[GATE — never request write scopes]**
  - [ ] `access_type=offline`, `prompt=consent`, `include_granted_scopes=true` set correctly (`12_gmail_connector.md` §4.2)
- [ ] `adapter/postgres/repository.go` + sqlc queries for `users`, `oauth_tokens`
- [ ] `adapter/http/handler.go`: `POST /auth/google/initiate`, `GET /auth/google/callback` (`09_api_specification.md` §5.1)
- [ ] `module.go` composition

### Encryption & Session Security
- [ ] KMS integration stood up; envelope encryption implemented for `oauth_tokens` **[GATE]** (`16_security.md` §4.3)
- [ ] Platform session JWT issuance (short-lived, KMS-managed signing key) (`16_security.md` §5.1)
- [ ] Opaque, DB-backed, revocable refresh token implementation (`16_security.md` §5.1)
- [ ] OAuth `state` CSRF parameter: random, single-use, short-TTL (`16_security.md` §5.2)

### RBAC Scaffolding
- [ ] `user` role claim model implemented
- [ ] `ops` role claim model implemented (no endpoints use it yet — scaffolding only)
- [ ] Row-level ownership-check convention established: every user-scoped repository method signature requires `userID` as a mandatory parameter **[GATE]** (`16_security.md` §6.2, `20_coding_convention.md` §9.2)

### Database Schemas for Other Modules (Scaffolding Only)
- [ ] Goose migrations: `ingestion.gmail_accounts`, `ingestion.ingested_messages`, `ingestion.sender_watch_log` (tables only, not yet consumed by code)
- [ ] Goose migrations: `understanding.sender_templates`, `understanding.extraction_results`
- [ ] Goose migrations: `financialcore.financial_events`, `event_overrides`, `category_assignments`, `inbox_processed` **[GATE — get this right now, Sprint 6 depends on it]**
- [ ] Goose migrations: `budget.budgets`, `budget_fired_alerts`, `savings_goals`
- [ ] Goose migrations: `analytics.*` views, `projection_checkpoint`
- [ ] Goose migrations: `engagement.notification_jobs`, `notification_preferences`
- [ ] Every table reviewed against `08_database_design.md`'s constraint list — no cross-schema FK anywhere **[GATE]**

### Testing
- [ ] Unit tests: `User`/`OAuthToken` domain invariants
- [ ] Integration test: full OAuth link flow against a mocked Google OAuth endpoint
- [ ] Integration test: duplicate-email-linking conflict returns `409`, no duplicate row (H1, per `07_sequence_diagrams.md` Diagram 1)

### Frontend (Parallel Track)
- [ ] Frontend project scaffolded (React + Vite), consuming `09_api_specification.md` exclusively — no direct DB/API-provider access **[GATE]**
- [ ] API client wrapper with `Authorization: Bearer` session-token handling, decimal-string money parsing (never `parseFloat` on money fields — match the backend's decimal-string convention)
- [ ] Landing/login page: "Sign in with Google" → `POST /auth/google/initiate` → redirect
- [ ] OAuth callback page: handles `GET /auth/google/callback` response, stores session token, redirects to app shell
- [ ] Session refresh handling (silent refresh before access-token expiry, redirect to login on refresh failure)
- [ ] Basic app shell/layout (nav, empty dashboard placeholder) — nothing functional yet beyond auth
- [ ] Error boundary + a consistent API-error-envelope display component (matches `09_api_specification.md` §7.2's shape) — build this once, reuse every sprint

### Observability
- [ ] Structured logging (`slog`) wired at the composition root per `06_architecture.md` §10 — `identity` module logs include `requestId`/`traceId`
- [ ] OpenTelemetry tracer initialized; root span created on every HTTP request
- [ ] Frontend: basic error/exception reporting wired (e.g., Sentry or equivalent) — not optional, catch auth-flow failures from day one

---

## Exit Criteria

- [ ] A user can complete Google OAuth sign-in end to end locally and receive a platform session token
- [ ] Tokens are verifiably encrypted at rest (inspect the DB directly — ciphertext, not plaintext)
- [ ] All 8 schemas exist with their full MVP table set, reviewed against `08_database_design.md`
- [ ] `identity` module passes `make lint`, `make test-unit`, `make test-integration` cleanly
- [ ] Module-boundary linter confirms `identity` has zero forbidden cross-schema/cross-module access
- [ ] Frontend: user can click "Sign in with Google" in a browser, complete consent, and land on an authenticated empty app shell
