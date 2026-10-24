# Backend Blueprint
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `04_domain_model.md`, `06_architecture.md`, `08_database_design.md`, `09_api_specification.md`
**Feeds Into:** `21_project_structure.md`, `20_coding_convention.md`, `11_ai_blueprint.md`, `12_gmail_connector.md`

**Naming Convention Note:** Per project convention, Go identifiers (variables, struct fields, function names) use **camelCase**/**PascalCase** (Go's standard export-casing convention layered on top of camelCase), matching the JSON wire format from `09_api_specification.md`. Database identifiers remain snake_case per `08_database_design.md`; sqlc's generated struct field names are the explicit translation boundary between the two.

---

## 1. Purpose

This document specifies **how the Go backend is actually built** — not line-by-line code (explicitly out of scope per the project brief), but the concrete implementation patterns, package responsibilities, handler shapes, and code-organization rules that every module (`identity`, `ingestion`, `understanding`, `financialcore`, `budget`, `analytics`, `engagement`, `administration`) must follow to remain consistent with the Clean Architecture layering and module boundaries established in `06_architecture.md`.

Where `06_architecture.md` says "each module has Domain, Application, and Adapter layers," this document says precisely what lives in each layer, how they're wired, and what a command handler, query handler, and Asynq task handler each look like in this codebase's idiom.

## 2. Scope

### 2.1 In Scope
- Per-module internal package structure (Domain / Application / Adapter breakdown).
- Command handler pattern (write-side use cases).
- Query handler pattern (read-side use cases, including CQRS read-side for Analytics).
- Asynq task handler pattern, including Outbox Relay and Inbox-guard implementation shape.
- sqlc query organization and repository interface pattern.
- Error handling and result-type conventions.
- HTTP handler-to-application-layer wiring (chi integration).
- Validation strategy (`go-playground/validator` placement in the pipeline).

### 2.2 Out of Scope
- Actual Go source code / function bodies (per the project's "never generate implementation code unless explicitly requested" directive).
- Full directory tree listing → `21_project_structure.md`.
- Style/formatting rules (naming, linting specifics) → `20_coding_convention.md`.
- AI-specific extraction pipeline internals → `11_ai_blueprint.md`.

---

## 3. Architecture: Per-Module Internal Structure

Every module follows an identical internal shape, so that any engineer familiar with one module can navigate any other module without relearning conventions:

```
<module>/
├── domain/
│   ├── aggregate.go        (aggregate root + entities + invariants)
│   ├── valueobject.go      (value objects: Money, Confidence, Provenance, etc.)
│   ├── event.go            (domain event type definitions)
│   └── repository.go       (repository INTERFACES only — no implementation)
│
├── application/
│   ├── command/
│   │   └── <verb><noun>.go (one file per command handler, e.g., finalizeFinancialEvent.go)
│   ├── query/
│   │   └── <verb><noun>.go (one file per query handler)
│   └── policy/
│       └── on<Event>.go    (one file per reactive policy, e.g., onMessageIngested.go)
│
├── adapter/
│   ├── http/
│   │   ├── handler.go      (chi handler funcs, DTO mapping)
│   │   └── dto.go          (request/response DTOs matching 09_api_specification.md)
│   ├── task/
│   │   └── handler.go      (Asynq task handler funcs)
│   ├── postgres/
│   │   ├── repository.go   (repository interface implementations using sqlc)
│   │   └── queries/        (sqlc .sql source files, scoped to this module's schema only)
│   ├── outbox/
│   │   └── relay.go        (this module's Outbox Relay poller, if it owns one)
│   └── external/           (only present in modules with external dependencies,
│                             e.g., understanding/adapter/external/llmclient.go,
│                             ingestion/adapter/external/gmailclient.go)
│
└── module.go               (NewModule constructor — composition root entry point per 06_architecture.md §8)
```

**Rule:** `domain/` imports nothing from `application/` or `adapter/`. `application/` imports only from this module's own `domain/` and from narrowly published cross-module interfaces (Section 6). `adapter/` implements `domain/repository.go` interfaces and wires into `application/` via `module.go`. This is the literal, file-level enforcement of the Dependency Rule from `06_architecture.md` Section 3.2.

---

## 4. Command Handler Pattern (Write-Side Use Cases)

Every state-changing operation — whether triggered by an HTTP request or a reactive Policy — is implemented as a **Command Handler**: a small, single-purpose type with one public method, taking a Command struct and returning a Result or error.

**Shape (described, not coded):**
- A `Command` struct (e.g., `FinalizeFinancialEventCommand`) carrying only the data needed to execute the use case — never a raw HTTP request or database row.
- A `Handler` struct holding only the repository interfaces and domain services it needs (constructor-injected, per `06_architecture.md` Section 8's manual DI approach).
- A single `Handle(ctx, command) (Result, error)` method that:
  1. Loads any necessary aggregate(s) via the repository interface.
  2. Invokes domain-layer methods to enforce invariants (e.g., `financialEvent.Finalize(confidence, threshold)` internally checks INV-1 from `04_domain_model.md` and returns a domain error if violated).
  3. Persists the change **and** the corresponding Outbox row within a single repository-provided transaction (the repository interface exposes a `WithTx` or equivalent unit-of-work method — this is where the Outbox pattern's atomicity requirement from `06_architecture.md` Section 6.1 is structurally enforced at the code level, not left to handler discipline).
  4. Returns a Result DTO (never a raw domain aggregate — keeps the Application layer's public surface stable even if domain internals evolve).

**Example command handlers per module (illustrative list, not exhaustive):**

| Module | Command Handlers |
|---|---|
| `identity` | `LinkGoogleAccount`, `RefreshOAuthToken`, `UpdateUserSettings` |
| `ingestion` | `StartInitialFullSync`, `RegisterGmailWatch`, `RunIncrementalSync`, `DisconnectGmailAccount` |
| `understanding` | `ClassifyAndRouteMessage`, `ApplySenderTemplate`, `InvokeAIExtractor` |
| `financialcore` | `EvaluateConfidenceGate`, `ConfirmPendingReviewEvent`, `SubmitEventOverride`, `ArchiveFinancialEvents` |
| `budget` | `CreateBudget`, `EvaluateBudgetImpact`, `CreateSavingsGoal` |
| `engagement` | `ComposeNotification`, `DeliverViaWhatsApp` |
| `administration` | (primarily event-subscriber driven; minimal direct commands — see Section 7) |

---

## 5. Query Handler Pattern (Read-Side Use Cases)

Queries are kept structurally distinct from commands (a light CQRS separation applied uniformly, even within modules that aren't the dedicated Analytics read-side) because read paths have fundamentally different performance and consistency needs.

**Shape:**
- A `Query` struct (e.g., `ListFinancialEventsQuery{ UserID, From, To, Status, Cursor, Limit }`).
- A `Handler` with a single `Handle(ctx, query) (Result, error)` method that calls a **read-optimized repository method** (often a dedicated sqlc query distinct from the one used by command handlers, even against the same table, to allow independent optimization — e.g., a list query selects only the columns needed for the list DTO, not the full aggregate).
- Query handlers **never** invoke domain aggregate methods that could mutate state — they are read-only by construction (enforced by convention and code review, since Go does not have a compiler-level read-only-repository mechanism; this is called out explicitly in `20_coding_convention.md`'s review checklist).

**Analytics module specifically:** Every query handler in `analytics` reads exclusively from that module's own schema (Section 11 of `08_database_design.md`), never reaching into `financialcore`'s tables — this is both a module-boundary rule (`06_architecture.md` Section 5) and a query-handler-pattern rule reinforced here for emphasis, since it's the single most important boundary to never accidentally violate for performance-shortcut reasons.

---

## 6. Cross-Module Interface Pattern

Per `06_architecture.md` Section 5.1.2, a module may publish a narrow interface for another module's synchronous use. The pattern:

- The **providing** module defines the interface in its own `domain/` or a small dedicated `port.go` file (e.g., `identity` defines `TokenProvider` with a single method, `GetValidAccessToken(ctx, gmailAccountID) (string, error)`).
- The **providing** module implements it in its own `adapter/` layer.
- The **consuming** module's `application/` layer declares its own dependency on that interface type (imported from the providing module's package — this is the one sanctioned cross-module import, limited strictly to interface types, never concrete structs).
- Wiring happens at the composition root (`main.go`), which constructs the providing module first and passes its interface implementation into the consuming module's constructor.

**Enumerated sanctioned cross-module interfaces (initial set):**

| Interface | Provided By | Consumed By | Purpose |
|---|---|---|---|
| `TokenProvider` | `identity` | `ingestion` | Fetch a valid, refreshed Gmail access token for API calls |
| `AnalyticsSnapshotReader` | `analytics` | `engagement` | Fetch composed summary data for notification rendering (read-only, narrow) |
| `UserPreferenceReader` | `identity` | `engagement`, `budget` | Check notification opt-in status / timezone before composing/scheduling |

Any new cross-module interface must be added to this table before implementation, keeping this document the single enumeration of all sanctioned boundary crossings — a new interface appearing in code without a corresponding entry here should fail review.

---

## 7. Policy Handler Pattern (Reactive Handlers)

Policies from `05_event_storming.md` (e.g., *"When `FinancialEventFinalized` → issue `EvaluateBudgetImpact`"*) are implemented as **Asynq task handlers** that:

1. Are registered against a specific queue (per the topology in `06_architecture.md` Section 6.3) and task type name matching the domain event that triggers them.
2. Deserialize the task payload (the Outbox event's `payload` JSON) into a typed Go struct.
3. Apply the Inbox-pattern guard (Section 8) if the resulting side effect isn't already naturally idempotent.
4. Translate the event into the corresponding Command and invoke that module's own Command Handler (Section 4) — **policies never contain business logic themselves**; they are thin translators from "an event happened" to "issue this command," keeping all actual domain logic inside Command Handlers where it's more directly unit-testable without a queue in the loop.

This "policy is a thin adapter, not a business-logic container" rule is a deliberate architectural discipline: it means every reactive behavior in the system is testable two ways — (a) unit-test the Command Handler directly with a hand-built Command, no queue needed, and (b) a smaller set of integration tests verifying the Policy correctly translates the expected event shape into that Command. This split is directly reflected in the test pyramid defined in `18_testing_strategy.md`.

---

## 8. Outbox Relay & Inbox Guard Implementation Shape

### 8.1 Outbox Relay

Each module that owns an `outbox_events` table (per `08_database_design.md` Section 4) runs a dedicated background goroutine (started in `worker` mode only, per `06_architecture.md` Section 11's dual-mode binary) that:
- On a fixed interval (configurable via Viper, default sub-second to a few seconds), executes the `SELECT ... WHERE relayed = false ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT N` query against its own `outbox_events` table.
- For each row, enqueues the corresponding Asynq task (task type derived from `event_type`, payload from `payload`).
- On successful enqueue, updates `relayed = true, relayed_at = now()` in the same transaction scope as the row lock (using `SKIP LOCKED` specifically so multiple relay instances — one per worker replica — never process the same row twice, providing safe horizontal scaling of the relay itself).

### 8.2 Inbox Guard

Implemented as a small shared helper (a single reusable internal package, e.g., `internal/inboxguard`, imported by any module's task handlers that need it) providing a single function shape: *"given a transaction and a task ID, check-and-claim; return whether this is a first-time or duplicate delivery."* This avoids each module reimplementing the same check/insert pattern independently and drifting in subtle ways — the single shared helper is the canonical implementation referenced by `06_architecture.md` Section 6.2.

---

## 9. sqlc Query Organization

- Each module's `adapter/postgres/queries/` directory contains `.sql` files, one per logical query group (e.g., `financial_events.sql`, `event_overrides.sql`), following sqlc's annotation convention (`-- name: GetFinancialEventByID :one`, etc.).
- sqlc is configured **per module** (a `sqlc.yaml` scoped to each module's schema and query directory, or a single root `sqlc.yaml` with per-module `sql`/`queries` path entries — the latter is preferred for a single generated-code root, but each module's generated package remains import-isolated per the boundary rule).
- Generated Go structs/methods land in a module-scoped generated package (e.g., `financialcore/adapter/postgres/gen/`), never a shared cross-module generated package — reinforcing that no module can accidentally get type-level access to another module's tables just because sqlc generated code for them.
- **Command-side vs. query-side queries are written as separate `.sql` files** even when touching the same table, per Section 5's read/write separation discipline (e.g., `financial_events_write.sql` containing the `INSERT ... ON CONFLICT DO NOTHING` used by command handlers, vs. `financial_events_read.sql` containing the paginated list/detail queries used by query handlers).

---

## 10. Repository Interface Pattern

Each module's `domain/repository.go` defines interfaces expressed entirely in domain terms (no `sql.Tx`, no sqlc-generated types leaking into the signature):

- Example shape: a `FinancialEventRepository` interface exposing methods like `Save(ctx, event) error`, `FindByID(ctx, id) (*FinancialEvent, error)`, `ExistsBySourceRef(ctx, connectorAccountID, sourceMessageRef) (bool, error)` — all operating on the `domain.FinancialEvent` aggregate type, never a raw DB row struct.
- The `adapter/postgres/repository.go` implementation translates between the domain aggregate and sqlc-generated row structs internally — this translation layer is precisely where snake_case (DB) meets camelCase/PascalCase (Go domain) meets the JSON DTO's camelCase (API layer, a separate translation happening in `adapter/http/dto.go`). Three distinct translation boundaries are kept intentionally separate (DB↔Domain, Domain↔DTO) rather than collapsed into one "do everything" mapping function, so that a schema change and an API contract change can each be handled independently without touching unrelated code.

---

## 11. HTTP Handler Wiring (chi Integration)

- Each module's `adapter/http/handler.go` exposes a `RegisterRoutes(router chi.Router)` function, called once from the composition root, mounting the module's routes under its `09_api_specification.md`-defined path prefix.
- Standard middleware chain (applied globally at the root router, not per-module, to guarantee consistency): request-ID injection → OpenTelemetry span start → authentication (session token validation) → `go-playground/validator`-based request DTO validation (via a generic decode-and-validate middleware helper) → module handler.
- Handler functions themselves are thin: decode already-validated DTO → construct Command/Query → invoke the Application-layer handler → map Result to Response DTO → write JSON. No business logic, no direct repository access, ever, at this layer.

---

## 12. Error Handling Convention

- Domain layer defines its own sentinel/typed errors (e.g., `domain.ErrConfidenceBelowThreshold`, `domain.ErrCurrencyMismatch` for INV-2 violations) — these are meaningful, specific error values, not generic `errors.New("something went wrong")`.
- Application layer wraps and propagates these using Go's `%w` error wrapping, adding context without discarding the original typed error, so a caller (including the HTTP adapter) can `errors.Is`/`errors.As` to determine the correct HTTP status mapping (Section 7.2 of `09_api_specification.md`).
- A single, shared `adapter/http` helper translates known domain error types to the API's standard error envelope and status codes (e.g., `ErrConfidenceBelowThreshold` → this specific error should never actually surface to an HTTP client directly, since gating happens asynchronously — but for synchronous validation errors like `ErrCurrencyMismatch` on an override submission, the mapping is `422` per `09_api_specification.md` Section 7.2). Unmapped/unexpected errors default to `500` with the underlying error logged (never leaked to the client response body) — full detail in `20_coding_convention.md`.

---

## 13. Detailed Explanation: Why Commands, Queries, and Policies Are Physically Separate Packages

A common shortcut is to put all "service" logic for a module into one `service.go` file with a grab-bag of methods. This blueprint deliberately splits into `command/`, `query/`, and `policy/` sub-packages because:

1. **Testability boundary:** Command handlers are the unit under test for business-rule correctness; Query handlers are the unit under test for correct read-shaping; Policy handlers are the (much smaller) unit under test for correct event-to-command translation. Conflating them into one file makes it easy to accidentally let a query handler perform a write "just this once," silently reintroducing the exact CQRS-boundary erosion this architecture is designed to prevent.
2. **Onboarding legibility:** A new engineer asking "what write operations does the `budget` module support" can answer it by listing files in `budget/application/command/`, without reading implementation bodies.
3. **This structure directly mirrors the event-storming artifact** (`05_event_storming.md`): every 🟦 Command, 🟪 Policy in that document has an exact, findable corresponding file here, keeping the documentation set and the codebase in permanent, navigable correspondence.

---

## 14. Component Responsibilities (Implementation-Level)

| Component | Responsibility |
|---|---|
| `domain/` package | Aggregate/entity/value-object definitions, invariant enforcement, domain events, repository interfaces |
| `application/command/` | Write-side use case orchestration; the only place business transactions are committed |
| `application/query/` | Read-side use case orchestration; strictly non-mutating |
| `application/policy/` | Thin event-to-command translators; no business logic |
| `adapter/http/` | HTTP protocol concerns and DTO mapping only |
| `adapter/task/` | Asynq protocol concerns; delegates to `application/policy/` |
| `adapter/postgres/` | Repository interface implementations via sqlc; DB↔Domain translation |
| `adapter/outbox/` | This module's Outbox Relay poller (present only in modules that emit events consumed by others) |
| `adapter/external/` | Third-party API client wrappers (Gmail, WhatsApp, LLM provider), implementing domain-defined gateway interfaces |
| `module.go` | Composition root for this module: wires Domain → Application → Adapter, exposes route/task registrars |

---

## 15. Trade-offs

| Decision | Trade-off |
|---|---|
| Physical command/query/policy package separation | More files and initial ceremony per module, in exchange for a codebase structure that stays legible and hard to accidentally erode as the system grows and more engineers contribute. |
| Manual DI via constructor injection (no framework) | More explicit composition-root code, in exchange for compile-time-checked, fully traceable dependency graphs — especially valuable when reasoning about the cross-module interface table (Section 6). |
| Separate read/write sqlc query files even for the same table | Minor duplication of simple `SELECT`-shape queries across files, in exchange for independent evolution of read and write query performance characteristics without cross-contamination. |
| Shared `internal/inboxguard` helper vs. per-module reimplementation | A small amount of shared internal-package coupling (acceptable, since it's a generic infrastructure helper, not domain logic) in exchange for consistent idempotency-guard behavior platform-wide. |

## 16. Advantages
- The one-to-one correspondence between this blueprint's package structure and the Event Storming document's Command/Event/Policy vocabulary makes the codebase self-documenting relative to the design series.
- Strict command/query/policy separation directly operationalizes the CQRS-where-beneficial principle from the Vision without requiring every module to adopt full CQRS machinery.
- The three-boundary translation discipline (DB↔Domain, Domain↔DTO) keeps schema evolution and API evolution decoupled — a recurring pain point in less disciplined codebases.

## 17. Disadvantages
- The package-per-concern structure produces more files per module than a minimal "everything in one service file" approach — a deliberate, accepted cost for a financial system prioritizing long-term maintainability over initial file count.
- Manual DI at the composition root will require care as the cross-module interface table (Section 6) grows; without discipline, `main.go` could become an unwieldy wiring file — mitigated by keeping each module's own `NewModule` constructor responsible for its *internal* wiring, so `main.go` only handles the *cross-module* wiring surface, which stays proportionally smaller.

## 18. Future Improvements
- If the `understanding` module is extracted into a standalone service (per `06_architecture.md` Section 18's noted first-extraction-candidate), this blueprint's adapter-layer separation (especially `adapter/external/llmclient.go` and `adapter/task/handler.go`) is specifically designed so the extraction primarily involves moving the module directory and swapping its Outbox/Inbox transport binding from in-process Asynq to a network-based equivalent, with `domain/` and `application/` needing minimal to no change.
- Consider introducing a lightweight code-generation step (from this blueprint's conventions) for boilerplate command/query handler scaffolding, if the file-count overhead becomes a measured friction point for the team.

## 19. Best Practices
- Every Command Handler must have a corresponding domain-layer invariant check reachable via its aggregate method — command handlers must never re-implement invariant logic inline; invariants live in `domain/`, not `application/`.
- No `adapter/` package may be imported by `domain/` or `application/` — this must be a CI-enforced rule (`06_architecture.md` Section 5.3), not merely a code-review convention.
- Every new Policy handler must be added to both this document (Section 4/7's tables) and cross-checked against `05_event_storming.md`'s Policy list to avoid drift between the two documents.

## 20. Production Considerations
- The Outbox Relay's poll interval (Section 8.1) directly trades off event-propagation latency against database load from multiple worker replicas polling concurrently — `SKIP LOCKED` prevents duplicate processing but not redundant polling load, which must be tuned and monitored per `06_architecture.md` Section 20's production guidance.
- Repository interface method signatures should be designed defensively against N+1 query patterns from day one (e.g., a `ListFinancialEvents` query method must support batched loading of related `CategoryAssignment` data in one query, not one query per returned item) — this is a code-review-enforced discipline, not automatically prevented by the architecture, and should be explicitly called out in `20_coding_convention.md`.
- Structured error types (Section 12) must be kept stable across releases where they cross the HTTP boundary — changing an error code's meaning is a breaking API change subject to the versioning policy in `09_api_specification.md`.

---

**Next Document:** `11_ai_blueprint.md` — specifies the AI Engine's internal design in full: prompt architecture and versioning, structured-output schema enforcement, confidence-score computation methodology, categorization strategy, and the provider-agnostic client abstraction referenced in this blueprint's `adapter/external/llmclient.go`.
