# Security
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `03_software_requirement_specification.md`, `04_domain_model.md`, `06_architecture.md`, `08_database_design.md`, `09_api_specification.md`, `11_ai_blueprint.md`, `12_gmail_connector.md`, `14_workflow_engine.md`
**Feeds Into:** `17_observability.md`, `18_testing_strategy.md`, `19_deployment.md`, `22_risk_analysis.md`

---

## 1. Purpose

This document consolidates and formalizes the platform's complete security architecture: OAuth token encryption and key management, RBAC for the `/admin/*` surface, PII-minimization and data-handling controls (referenced throughout `11_ai_blueprint.md` and `15_notification_system.md`), GDPR compliance mechanics (referenced throughout `01_product_vision.md`, `03_software_requirement_specification.md`, and `07_sequence_diagrams.md`'s purge cascade), and the audit-logging design underpinning every module's compliance obligations.

This document does not introduce new security *principles* not already referenced elsewhere in the series — its purpose is to be the **single authoritative, consolidated specification** that a security review, a penetration test scope, or a compliance audit can be run against, rather than requiring the security posture to be reconstructed by reading fragments scattered across thirteen other documents.

## 2. Scope

### 2.1 In Scope
- Encryption at rest and in transit specification.
- Key management (KMS integration, envelope encryption detail for OAuth tokens).
- Authentication and session security (expanding `09_api_specification.md` Section 5).
- RBAC model for internal/admin access.
- PII classification and handling rules (consolidating `11_ai_blueprint.md` Section 9, `08_database_design.md`'s retention notes).
- GDPR compliance mechanics: data export, deletion/purge cascade verification, consent record-keeping.
- Audit logging design and its own security properties (tamper-resistance, retention).
- Vulnerability and dependency management process.
- Secrets management.

### 2.2 Out of Scope
- Network-level infrastructure hardening detail (firewall rules, VPC design) → `19_deployment.md`.
- Specific penetration-testing findings/results (a post-implementation operational activity, not a design artifact).

---

## 3. Architecture: Security Layering Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 1: Network / Edge (Traefik, TLS termination, WAF-level    │
│  rate limiting) — detailed jointly here and in 19_deployment.md   │
├─────────────────────────────────────────────────────────────┤
│  Layer 2: Authentication & Session (Google OAuth, platform        │
│  session tokens, RBAC claims)                                     │
├─────────────────────────────────────────────────────────────┤
│  Layer 3: Application-Level Authorization (per-module ownership    │
│  checks, admin-role gating, module-boundary enforcement per         │
│  06_architecture.md §5)                                             │
├─────────────────────────────────────────────────────────────┤
│  Layer 4: Data Protection (encryption at rest — envelope             │
│  encryption for tokens, TLS in transit, PII minimization)             │
├─────────────────────────────────────────────────────────────┤
│  Layer 5: Audit & Compliance (append-only audit log, GDPR             │
│  export/deletion mechanics, consent tracking)                          │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. Encryption Specification

### 4.1 In Transit

- **TLS 1.2+ mandatory** for all external traffic (client↔Traefik, Traefik↔API instances if not co-located, platform↔Gmail API, platform↔WhatsApp Business API, platform↔LLM provider) — no exceptions, no plaintext fallback permitted at any hop.
- Internal service-to-service traffic (API instances ↔ PostgreSQL, ↔ Redis) uses TLS where the deployment topology (`19_deployment.md`) places these across a network boundary that isn't otherwise physically/logically isolated — minimum a documented, reviewed decision per environment, not a default assumption of "internal is safe."

### 4.2 At Rest — General

- PostgreSQL: encryption at rest via the underlying storage/volume layer (cloud provider-managed disk encryption or equivalent), covering all schemas uniformly as a baseline.
- **OAuth tokens receive an additional, application-level encryption layer beyond baseline disk encryption** (Section 4.3) — this is the one class of data in the system warranting defense-in-depth beyond infrastructure-level encryption, since a token is a direct, high-value credential to a user's live Gmail mailbox, not merely sensitive business data.

### 4.3 Envelope Encryption for OAuth Tokens

Per `08_database_design.md` Section 6.2's `encrypted_access_token`/`encrypted_refresh_token` columns:

1. A **Key Management Service (KMS)**-managed root key (cloud-provider KMS or an equivalent self-hosted HSM-backed solution, per deployment environment) never leaves the KMS boundary.
2. Each token is encrypted using a **per-record Data Encryption Key (DEK)**, itself encrypted ("wrapped") by the KMS root key — the wrapped DEK is stored alongside the ciphertext in `oauth_tokens`; the root key is never directly used to encrypt token data itself (standard envelope-encryption practice, chosen specifically because it allows key rotation, Section 4.4, without re-encrypting every token record individually).
3. Decryption requires a live KMS call (unwrap the DEK using the current root key) for every token use — meaning a KMS outage degrades token refresh capability (a documented failure mode, addressed in `06_architecture.md` Section 12's failure-domain table as an extension: KMS unavailability should be added there as an ingestion-blocking dependency) but never exposes token material even under a database-only compromise.

### 4.4 Key Rotation

- KMS root key rotation is supported natively by envelope encryption's design (Section 4.3) — rotating the root key requires only re-wrapping existing DEKs with the new root key (a lightweight batch operation), never re-encrypting the underlying token ciphertext itself.
- Rotation cadence: minimum annual, or immediately upon suspected compromise — a documented, calendared operational process (`19_deployment.md`), not a one-time setup step.

---

## 5. Authentication & Session Security

### 5.1 Platform Session Tokens

Expanding `09_api_specification.md` Section 5.2:
- Short-lived JWTs (default 1-hour access lifetime), signed with an asymmetric key (allowing signature verification without distributing a shared secret to every consuming component) — the signing key itself is KMS-managed, consistent with Section 4.3's principle.
- A separate, longer-lived, opaque (non-JWT, database-backed, revocable) **refresh token** is used to obtain new session JWTs — chosen specifically because opaque, database-backed refresh tokens can be **immediately revoked** (e.g., on suspected compromise or explicit logout) in a way that a long-lived JWT cannot be, without a separate revocation-list mechanism.
- Session refresh tokens are themselves subject to the same class of protection as Section 4.3 (encrypted at rest), since they are also a direct account-access credential.

### 5.2 CSRF Protection

- The OAuth `state` parameter (`09_api_specification.md` Section 5.1) is a cryptographically random, single-use, short-TTL value validated on callback — standard OAuth CSRF mitigation.
- All state-changing platform API endpoints require the `Authorization: Bearer` header (not cookie-based session auth), which is inherently not subject to traditional browser CSRF attack vectors (no ambient credential automatically attached by the browser) — a deliberate authentication-mechanism choice that sidesteps an entire class of CSRF concerns for the platform's own API, distinct from the OAuth flow's own CSRF protection which remains necessary regardless.

### 5.3 Rate Limiting

- Edge-level (Traefik) rate limiting on `/auth/*` endpoints specifically, to mitigate credential-stuffing/brute-force-style abuse even though the platform has no password to brute-force (OAuth-only) — the OAuth *initiation* and *callback* endpoints are still abuse targets for enumeration/resource-exhaustion attacks.
- `/webhooks/*` endpoints have rate limiting tuned against expected legitimate provider call volume, with anomalous spikes alerting (`17_observability.md`) rather than silently absorbing potentially malicious traffic.

---

## 6. RBAC Model

### 6.1 Roles

| Role | Scope | Assigned To |
|---|---|---|
| `user` | Full access to own resources only (`/me/*`, `/financial-events/*` scoped to `user_id`, `/budgets/*`, `/notifications/preferences`) | Every authenticated platform user, default |
| `ops` | Read-only access to `/admin/*` endpoints (`09_api_specification.md` Section 6.6) | Internal operations/engineering staff, provisioned individually, never shared credentials |
| `ops-write` | (Reserved, not currently used by any endpoint per `09_api_specification.md`'s current catalog being entirely read-only under `/admin/*`) — explicitly not granted to anyone at this stage; any future admin write capability (e.g., a manual template force-deactivation tool) requires this distinct role, never silently added to `ops`'s read-only scope | N/A currently |
| `n8n-service` | Narrowly-scoped read-only subset of `ops`, per `14_workflow_engine.md` Section 5.2 | The n8n service credential exclusively |

### 6.2 Ownership Enforcement

Every `user`-scoped resource access is additionally checked against **row-level ownership** (`user_id` matches the authenticated session's user) at the Application layer (`10_backend_blueprint.md`'s Command/Query handlers) — this is enforced in application code (not solely relied upon via database RLS, though PostgreSQL Row-Level Security may additionally be enabled as defense-in-depth per `19_deployment.md`'s hardening checklist), and every query handler for user-scoped resources must include the ownership predicate as a **mandatory, non-optional parameter** in its repository interface signature (a code-review-enforced convention documented in `20_coding_convention.md`, preventing an accidental "list all events" query missing a `WHERE user_id = ?` clause).

### 6.3 Admin Access Provisioning

- `ops` role assignment is individual (never a shared account), tied to the operations staff member's own identity, and time-bounded where the organization's access-management tooling supports it (e.g., periodic re-certification).
- Every `ops`-role action (even read-only) is captured in the audit log (Section 8) — read access to sensitive aggregate data is itself an auditable event for a financial platform, not merely write actions.

---

## 7. PII Classification & Handling

### 7.1 PII Classification Tiers

| Tier | Examples | Handling Rule |
|---|---|---|
| **Tier 1 — Credentials** | OAuth tokens, session refresh tokens | Envelope-encrypted (Section 4.3), never logged, never sent to any third party including the LLM provider |
| **Tier 2 — Direct Identifiers** | Email address, display name | Encrypted at rest (baseline), never included in LLM prompts (per `11_ai_blueprint.md` Section 9), visible only to the owning user and `ops`-role audit-logged access |
| **Tier 3 — Financial Detail** | Transaction amounts, merchant names, masked payment method hints | Encrypted at rest (baseline), minimized before LLM prompt inclusion (masked account numbers per `11_ai_blueprint.md` Section 9), never shared cross-user except in the specifically privacy-preserving aggregated form (`11_ai_blueprint.md` Section 7's global category mapping) |
| **Tier 4 — Raw Email Content** | Full email body/headers | Time-boxed retention (`04_domain_model.md` Section 6.3, default 30 days), stored in object storage (MinIO) with access restricted to the `ingestion`/`understanding` modules' service credentials only — never directly queryable by `ops` role without an explicit, audited, justified access request process (not a standing dashboard capability) |

### 7.2 Cross-Reference to AI Prompt Minimization

Section 7's Tier 1/2 rules are the formal backing for `11_ai_blueprint.md` Section 9's PII-minimization design — that document specifies the *mechanics* (masking, stripping); this document specifies the *classification and policy rationale* those mechanics implement. Any future change to what's sent in an LLM prompt must be checked against this table's tier rules before implementation.

---

## 8. Audit Logging Design

### 8.1 What Is Logged

Per `04_domain_model.md` Section 12 and `08_database_design.md` Section 13.1, every context's state-changing domain events fan into `administration.audit_log`, including (non-exhaustive): OAuth connect/disconnect, manual `EventOverride` submissions, budget creation/modification, data purge requests and completions, `ops`-role read access to sensitive aggregate endpoints (Section 6.3), and template publish/rollback actions (`13_email_parser.md` Section 16).

### 8.2 Tamper-Resistance Properties

- The `administration_svc` database role has **no `DELETE` or `UPDATE` grant whatsoever** on `audit_log` — application code cannot modify or remove audit entries under any code path, by database-level permission design (`08_database_design.md` Section 13.1), not merely by application-logic discipline.
- Deletion, if ever legally mandated for a specific record (an edge case, e.g., a data-protection-authority order affecting even audit metadata), requires a separately-audited, superuser-executed, out-of-band process — never available to the application's normal operating credentials.
- Audit log writes are themselves part of each triggering transaction's Outbox-relayed fan-out (per `07_sequence_diagrams.md` Diagram 8), meaning audit entries are never lost to a crash between "action happened" and "action logged" — the same exactly-once-effect guarantee (`06_architecture.md` Section 6) applies to audit logging as to every other domain-event consumer.

### 8.3 Retention

Audit log entries are retained indefinitely by default (distinct from every other data category's time-boxed or user-controllable retention) — this is a deliberate compliance posture: the record that a deletion happened must outlive the deleted data itself, per `07_sequence_diagrams.md` Diagram 8's explicit exemption design. Any future jurisdiction-specific requirement to time-box even audit retention must be handled as an explicit, reviewed policy exception, not a default assumption.

---

## 9. GDPR Compliance Mechanics

### 9.1 Right to Access / Data Portability

A `GET /me/data-export` endpoint (an addition to `09_api_specification.md`'s catalog, formalized here since it is fundamentally a security/compliance-driven endpoint) triggers an asynchronous export job compiling: all `FinancialEvent` records (including override history), budget/savings-goal definitions, notification preferences, and account metadata — delivered as a downloadable, time-limited-access archive. Raw email content (Tier 4, Section 7.1) is **not** included in this export by default (it is transient supporting data, not the user's "financial event" record itself, per `04_domain_model.md` Section 6.3's boundary) — this scoping decision is explicitly documented here so it's a deliberate compliance-reviewed choice, not an oversight.

### 9.2 Right to Erasure

Directly implemented by the purge cascade specified in `07_sequence_diagrams.md` Diagram 8, with this document adding the formal compliance framing:
- The cascade must complete (or be demonstrably in verified progress) within the jurisdiction-mandated response window (commonly 30 days under GDPR) from the erasure request.
- The **audit log entry recording the erasure request and its completion is explicitly exempt** (Section 8.3) — necessary to demonstrate compliance was actually carried out, a common and accepted GDPR-compliant pattern (retaining minimal metadata about a deletion, not the deleted data itself).
- `18_testing_strategy.md` must include an automated, scheduled (not just pre-launch) compliance test verifying the full cascade's completeness against a synthetic test account, given the direct legal exposure of a partial-cascade bug (as flagged in `05_event_storming.md`'s H8 and `07_sequence_diagrams.md`'s note on the same).

### 9.3 Consent Record-Keeping

- The specific OAuth scopes granted at consent time are recorded (`08_database_design.md` Section 6.2's `scope` column) — providing an auditable record of exactly what the user agreed to at the time of linking, supporting both compliance verification and user-facing transparency (a user can be shown exactly what they consented to, matching `09_api_specification.md`'s `GET /me` response).
- Notification preference changes (opt-in/opt-out) are themselves audit-logged (Section 8.1), providing a consent trail for WhatsApp's own opt-out/consent expectations (`15_notification_system.md` Section 8) in addition to the platform's own GDPR posture.

---

## 10. Secrets Management

- All third-party credentials (Gmail OAuth client secret, WhatsApp Business API token, LLM provider API key, KMS access credentials themselves) are sourced exclusively from a dedicated secrets manager (cloud-provider secrets manager or equivalent, per `19_deployment.md`'s environment-specific detail) — **never** committed to the repository, never present in plaintext in any config file checked into version control, consistent with `06_architecture.md` Section 9's configuration-architecture rule.
- CI/CD pipeline (GitLab CI) secrets (deployment credentials, registry tokens) are managed via GitLab's own protected/masked CI variables, scoped to protected branches only, distinct from the runtime application secrets above.
- A secret-scanning pre-commit/CI check (e.g., detecting accidentally-committed API keys/tokens) is a mandatory pipeline stage, catching the most common real-world secrets-leakage failure mode before it reaches even a private repository history.

---

## 11. Vulnerability & Dependency Management

- Go module dependencies (and any Node/npm dependencies for tooling) are scanned via an automated dependency-vulnerability scanner integrated into the GitLab CI pipeline, blocking merges that introduce a known-critical-severity vulnerability without an explicit, documented, time-bounded exception.
- Container base images (Podman) are pinned to specific, regularly-updated versions (not `latest`), with image vulnerability scanning as a distinct CI stage from application-dependency scanning.
- A documented process exists for expedited patching of newly-disclosed critical vulnerabilities affecting any component in the stack (Go runtime, PostgreSQL, Redis, or any direct dependency) — detailed operationally in `19_deployment.md`'s incident-response runbooks, referenced here as a required capability.

---

## 12. Detailed Explanation: Why Row-Level Ownership Checks Live in Application Code, Not Solely in Database RLS

PostgreSQL's native Row-Level Security (RLS) could, in principle, enforce `user_id` ownership entirely at the database layer, removing the need for explicit application-code checks. This document specifies **application-layer enforcement as the primary, mandatory mechanism**, with database RLS positioned as optional defense-in-depth, because:

1. **Consistency with the module-boundary enforcement philosophy** (`06_architecture.md` Section 5.3): that section already establishes a pattern of *pairing* application-level and infrastructure-level controls (CI import-linting + database schema permissions) rather than relying on either alone — the same pairing philosophy applies here (explicit repository-interface-mandated ownership parameters + optional RLS), for the same reason: a single point of enforcement is a single point of failure if that specific layer is ever misconfigured or bypassed.
2. **RLS policies are easy to silently misconfigure or forget to apply to a new table** as the schema evolves (`08_database_design.md`'s schema grows over the product's life) — whereas a repository interface method signature that *requires* a `userID` parameter (Section 6.2) is a compile-time-visible, code-review-visible contract that's much harder to accidentally omit for a new query.
3. **Sqlc-generated code (per `10_backend_blueprint.md` Section 9) benefits more directly from explicit, hand-written `WHERE user_id = ?` clauses** that are visible in the reviewed `.sql` source files, than from an implicit RLS policy that isn't visible in the query file at all — favoring explicitness and reviewability, a recurring theme across this entire document series' technology choices (sqlc over a full ORM, manual DI over a framework, and now explicit ownership checks over implicit RLS-only enforcement).

---

## 13. Component Responsibilities

| Component | Responsibility |
|---|---|
| KMS | Root key custody for envelope encryption; never exposes raw key material |
| Session/Auth layer | JWT issuance/validation, refresh-token revocability, CSRF-resistant design |
| Application-layer ownership checks | Primary enforcement of per-user data isolation |
| `administration.audit_log` | Tamper-resistant, deletion-exempt compliance record |
| Secrets manager | Sole source of runtime credentials; never version-controlled |
| CI vulnerability/secret scanning | Automated, merge-blocking first line of defense against known-vulnerability and leaked-credential introduction |

---

## 14. Trade-offs

| Decision | Trade-off |
|---|---|
| Envelope encryption (KMS-dependent) for OAuth tokens | Adds a live KMS-availability dependency to every token-refresh operation, in exchange for token material that remains protected even under a full database compromise. |
| Application-layer-primary ownership enforcement (RLS as optional defense-in-depth, not primary) | Requires disciplined code review to ensure every new query includes the ownership predicate, in exchange for explicit, reviewable, sqlc-visible enforcement consistent with the rest of the platform's explicitness-favoring technology choices. |
| Indefinite audit log retention (vs. time-boxed like most other data) | Growing storage footprint over the platform's lifetime (a monitored, budgeted cost, not a design flaw), in exchange for durable compliance-evidence integrity. |
| Raw email content excluded from GDPR data export by default | Slightly narrower export scope than "everything we ever touched," in exchange for a scope that matches the platform's own data-model boundary (`04_domain_model.md` Section 6.3) between permanent financial record and transient supporting data — a defensible, documented interpretation, not an evasive one. |

## 15. Advantages
- This document gives a security reviewer or auditor one place to verify the platform's complete security posture against, rather than needing to reconstruct it from fragments across thirteen prior documents.
- The consistent "pair application-level and infrastructure-level enforcement" philosophy (Section 12) applied across module boundaries, ownership checks, and n8n credential scoping gives the platform genuine defense-in-depth, not single-layer security theater.
- GDPR mechanics are directly and traceably grounded in the domain model and sequence diagrams already designed (Diagram 8's cascade), meaning compliance isn't a bolted-on afterthought but a natural consequence of decisions made from `04_domain_model.md` onward.

## 16. Disadvantages
- The live-KMS-dependency for every token operation (Section 4.3) introduces an availability coupling that must be carefully monitored — a KMS regional outage could degrade Gmail ingestion platform-wide, a real operational risk that must be weighed against the security benefit and explicitly planned for in `19_deployment.md`.
- Indefinite audit log growth (Section 8.3) requires proactive storage capacity planning that a time-boxed-retention default would not — an accepted, monitored cost rather than an oversight, but one requiring ongoing operational attention.

## 17. Future Improvements
- Evaluate hardware security module (HSM)-backed KMS options if/when the platform's risk profile or a specific enterprise/regulatory customer requirement warrants a higher assurance level than a standard cloud KMS provides.
- Introduce field-level encryption (beyond the current table/column-level envelope encryption for tokens) for Tier 2 direct identifiers if a future compliance requirement (e.g., a specific regional data-protection law) mandates it — currently deemed sufficient via baseline at-rest encryption plus access control, but explicitly flagged as revisitable.
- Automate the GDPR erasure-cascade verification test (Section 9.2) as a continuously-scheduled synthetic-account job in production (not merely a pre-launch or CI-time test), providing ongoing, live compliance assurance rather than a point-in-time check.

## 18. Best Practices
- Every new table added to any schema (`08_database_design.md`) containing user-scoped data must have its corresponding repository interface methods reviewed for mandatory ownership-parameter inclusion (Section 12) before merge — a standing item in `20_coding_convention.md`'s PR checklist.
- No engineer or service, including `ops`-role holders, should ever have standing, un-audited access to Tier 4 raw email content (Section 7.1) — access must always flow through an explicit, justified, audit-logged request path.
- Secrets rotation (Section 4.4, Section 10) must be treated as a routine, calendared operational task, not an incident-triggered reactive one — waiting for a suspected compromise to first rotate a credential is a process failure, not a security control.

## 19. Production Considerations
- KMS availability must be added explicitly to `06_architecture.md` Section 12's failure-domain table as a production readiness action item stemming from this document — its absence there was noted in Section 4.3 and must be corrected before launch sign-off.
- The audit log's indefinite-retention storage growth must have a monitored, budgeted capacity-planning line item from launch, not discovered as a surprise cost months into operation.
- GDPR erasure-cascade completion times must be measured and monitored against the regulatory response-window requirement (Section 9.2) as a live production SLO, with alerting if any in-flight purge request approaches the deadline without completing — this is a compliance-critical production metric, not merely an engineering nicety.

---

**Next Document:** `17_observability.md` — specifies the full observability stack in detail: the complete Prometheus metric catalog (consolidating every metric referenced across prior documents — queue depth, projection lag, confidence calibration, parser failure rate, notification delivery tiers, quality-rating signals), Grafana dashboard design, OpenTelemetry tracing conventions, and alerting/SLO definitions.
