# Deployment
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `16_security.md`, `17_observability.md`, `18_testing_strategy.md`
**Feeds Into:** `22_risk_analysis.md`, `23_scalability.md`

---

## 1. Purpose

This document specifies the full deployment architecture: Podman container build/orchestration, Traefik routing/TLS configuration, the GitLab CI/CD pipeline (extending `18_testing_strategy.md` Section 10's test-gating into the full build-deploy pipeline), environment topology (dev/staging/production), and operational runbooks for the failure domains (`06_architecture.md` Section 12) and alerts (`17_observability.md` Section 8) catalogued earlier in the series.

This is the document that turns the entire design series into something that actually runs, stays running, and can be safely changed in production.

## 2. Scope

### 2.1 In Scope
- Container build strategy (Podman, multi-stage builds).
- Environment topology: dev, staging, production — promotion flow between them.
- GitLab CI/CD pipeline stages (build, test-gating per `18_testing_strategy.md`, deploy).
- Traefik configuration: routing, TLS, rate limiting at the edge.
- Database migration deployment process (Goose).
- Release strategy (rolling deploys, rollback mechanics).
- Operational runbooks for each failure domain and critical alert.
- Production-readiness checklist (consolidating gates from prior documents).

### 2.2 Out of Scope
- Capacity planning numbers/scaling thresholds themselves → `23_scalability.md`.
- Cloud-provider-specific infrastructure-as-code syntax (an implementation deliverable).

---

## 3. Architecture: Environment Topology

```
┌──────────┐      ┌──────────┐      ┌──────────┐
│    dev     │ ───▶ │  staging   │ ───▶ │ production │
│            │      │            │      │            │
│  per-branch│      │  shared,   │      │  shared,   │
│  ephemeral │      │  production│      │  live user │
│  (optional)│      │  -like     │      │  data      │
└──────────┘      └──────────┘      └──────────┘
```

- **dev:** Local developer environments (Podman Compose, matching `06_architecture.md` Section 11's dual-mode binary) plus optional ephemeral per-branch review environments for larger feature work.
- **staging:** A persistent, production-topology-matching environment (same module/schema layout, same Traefik/Asynq/Redis configuration shape, scaled down) used for integration validation, the GDPR cascade dry-run (`18_testing_strategy.md` Section 19), and pre-release sign-off. Uses synthetic/anonymized data (`18_testing_strategy.md` Section 11), never real user data.
- **production:** Live environment, serving real users, with the full observability and alerting stack (`17_observability.md`) active.

**Promotion flow:** Every change flows `dev → staging → production` strictly in order — no hotfix path skips staging, even under incident pressure, **except** for a narrowly-defined emergency-patch procedure (Section 9) that still requires the full blocking CI gate from `18_testing_strategy.md` Section 10, just with an expedited human-approval step.

---

## 4. Container Build Strategy (Podman)

### 4.1 Multi-Stage Build

- **Stage 1 (build):** Go build environment, compiles the single binary (per `06_architecture.md` Section 11's dual-mode design — one binary, `api` or `worker` mode selected at runtime via flag/env var, never two separately-built binaries) with build-time version/commit metadata embedded for traceability.
- **Stage 2 (runtime):** A minimal base image (distroless or Alpine-based, chosen for small attack surface per `16_security.md` Section 11's image-hardening posture), containing only the compiled binary and required runtime certificates — no Go toolchain, no source code, no build tools present in the final image.

### 4.2 Image Tagging & Versioning

- Images are tagged with the Git commit SHA (immutable, traceable) and, on release, additionally with a semantic version tag — `latest` is **never** used as a deployment target (per `16_security.md` Section 11's pinned-version discipline), only as a convenience alias for local development.
- Every production deployment references an exact, immutable image tag — enabling precise rollback (Section 8) to any prior known-good build.

### 4.3 Orchestration

- `podman-compose` (or an equivalent orchestration layer appropriate to the chosen production hosting environment) manages the multi-container topology: API instances, worker instances, Traefik, and — in non-managed-database deployments — PostgreSQL/Redis containers (with managed cloud database/cache services as the preferred production default where available, reducing operational burden versus self-hosting stateful services in containers).
- API and worker instance counts are independently configurable (per `06_architecture.md` Section 11's horizontal-scaling note), allowing the two to scale according to their distinct load profiles (HTTP request volume vs. queue depth).

---

## 5. GitLab CI/CD Pipeline

### 5.1 Pipeline Stages (Full, Extending `18_testing_strategy.md` Section 10)

```
lint → unit-test → integration-test → contract-test → ai-eval/template-eval
  → vulnerability-scan → build-image → [staging: deploy → smoke-test]
  → [production: manual-approval → deploy → smoke-test → monitor-window]
```

### 5.2 Deployment Stages Detail

- **Staging deploy:** Automatic on merge to main, after all `18_testing_strategy.md` Section 10 blocking stages pass. Runs database migrations (Section 6) automatically against the staging database.
- **Production deploy:** Requires an explicit **manual approval gate** in the GitLab CI pipeline (a named human approver, never auto-promoted from staging success alone) — a deliberate control given the financial-data stakes, consistent with the Vision's "never simplify the design" directive applied to release process, not only architecture.
- **Smoke tests:** A minimal, fast post-deploy check (health endpoint, a synthetic low-risk API call) run immediately after both staging and production deploys, before the deploy is considered complete — a failed smoke test triggers automatic rollback (Section 8), not a manual decision.
- **Monitor window:** Following a production deploy, an explicit, time-boxed (e.g., 30-minute) heightened-monitoring period during which the on-call engineer actively watches `17_observability.md`'s Platform Overview dashboard for anomalies before considering the deploy fully settled.

### 5.3 Secrets in CI

Per `16_security.md` Section 10: all CI-time secrets (registry credentials, deployment credentials) use GitLab's protected/masked CI variables, scoped to protected branches only — runtime application secrets (Gmail/WhatsApp/LLM credentials, KMS access) are **never** passed through CI variables at all; they are injected directly into the runtime environment from the production secrets manager, keeping the CI pipeline's secret surface area limited to deployment mechanics only.

---

## 6. Database Migration Deployment

- Goose migrations are applied as a **distinct pipeline step**, before the new application version's containers are rolled out — ensuring the schema is always compatible with (a superset of what) the incoming code expects, never the reverse ordering.
- **Backward-compatible migration discipline is mandatory:** every migration must be written such that the *previous* application version can still run correctly against the *post-migration* schema for the duration of a rolling deploy (Section 7) — e.g., adding a new nullable column is safe; dropping or renaming a column in the same migration that introduces its replacement is not (requires a two-phase migrate-then-cleanup-later approach instead). This is a `20_coding_convention.md`-enforced review rule for every migration PR.
- Per-module schema ownership (`06_architecture.md` Section 4) means migrations are organized and can, in principle, be deployed per-module — though in practice all migrations run as one Goose batch per deploy for operational simplicity at current scale, with the per-module organization preserved for future independent-deployability if modules are ever extracted (`06_architecture.md` Section 18).

---

## 7. Release Strategy

### 7.1 Rolling Deployment

- API instances: rolling replacement (new version instances brought up and health-checked before old version instances are drained and removed), ensuring zero-downtime deploys for the HTTP-serving layer.
- Worker instances: rolling replacement with **graceful shutdown** — a worker receiving a shutdown signal finishes its current in-flight task(s) before terminating (Asynq supports graceful shutdown natively), never abruptly killing a task mid-processing, which would otherwise rely purely on the Inbox-pattern's redelivery-safety (`06_architecture.md` Section 6.2) to recover — graceful shutdown is a courtesy that reduces unnecessary retry/redelivery churn, even though the system is already correct without it.

### 7.2 Rollback Mechanics

- Rollback = redeploying the immediately-prior known-good image tag (Section 4.2) — a fast, mechanical operation given immutable, SHA-tagged images.
- **Database migration rollback is explicitly NOT symmetric** with code rollback: per Section 6's backward-compatible discipline, a code rollback should remain compatible with the current (forward) schema state without needing a corresponding "down" migration in the common case — Goose "down" migrations exist and are maintained for development/testing convenience, but a production rollback runbook defaults to "roll back code only, leave schema forward" as the safer, well-tested path, only reaching for an actual schema-rollback migration in the rare case where the migration itself (not the application code) was the root cause of an incident.

---

## 8. Operational Runbooks (Per Failure Domain / Critical Alert)

Directly addressing `06_architecture.md` Section 12's failure-domain table and `17_observability.md` Section 8's critical alerts:

| Scenario | Runbook Summary |
|---|---|
| **LLM Provider unavailable** | Confirm via Prompt Monitoring dashboard; no immediate action required — `understanding:ai` queue absorbs backpressure per design (`06_architecture.md` §6.5); escalate only if outage exceeds a duration where the queue depth risks breaching SLA broadly; communicate expected delay to affected users via status page if prolonged. |
| **WhatsApp API unavailable** | Confirm via Engagement dashboard; `engagement:deliver` retries absorb short outages automatically; for prolonged outages, consider pausing non-critical notification types (e.g., `ai_recommendation`) to conserve messaging-tier budget for higher-priority alerts once service resumes. |
| **Gmail API rate-limited (429) platform-wide** | Check `ingestion_gmail_quota_units_consumed_total` against the project's granted quota; if near-exhausted, this is a capacity-planning failure (`23_scalability.md`), not a transient incident — file/expedite a quota-increase request with Google; in the interim, consider temporarily widening the poll-fallback interval (`12_gmail_connector.md` §5.3) to reduce consumption. |
| **PostgreSQL primary unavailable** | Full write-path outage — page immediately (Critical). Follow standard database failover procedure (specific mechanics depend on the chosen managed-database provider, documented in the provider-specific runbook appendix). Read-replica-served degraded Analytics read access may remain available if configured. |
| **Redis unavailable** | All async processing pauses (Critical page). Outbox rows remain safely durable in Postgres (`06_architecture.md` §6.1's explicit design guarantee) — communicate to stakeholders that no data loss is occurring, only a processing delay, while Redis is restored. |
| **KMS unavailable** | Token refresh operations begin failing (Critical page, per `16_security.md` §19). New Gmail API calls requiring a refreshed token will fail; already-valid, non-expired access tokens continue working until their natural expiry. Escalate to cloud-provider support if a provider-side KMS outage; this is explicitly the most severe failure-domain gap identified in `16_security.md` §4.3, warranting the fastest escalation path of any entry in this table. |
| **Poll-fallback safety net stale (H2 monitor)** | Treat as Critical per `17_observability.md` §8 — this is the system's own safety net failing silently; investigate the scheduled-task infrastructure itself (is the scheduler process alive? per-account task enqueueing succeeding?) before assuming push-notification health is the issue. |
| **Template failure-rate spike** | Not an incident in the traditional sense — a content/parser-maintenance task. Follow `13_email_parser.md` §6's authoring/validation workflow to produce and publish a fixed template version; monitor the AI-fallback queue depth in the meantime to ensure cost/throughput impact stays bounded. |
| **GDPR erasure approaching deadline** | Critical, paged with Compliance contact per `17_observability.md` §8. Manually verify the specific purge request's cascade state against `08_database_design.md`'s tables; if genuinely stuck (not merely slow), escalate as a compliance incident requiring possible direct regulatory communication, per legal/compliance process outside this document's engineering scope. |

---

## 9. Emergency Patch Procedure

For a genuinely urgent production fix (e.g., a security vulnerability actively being exploited, or a bug causing active financial-data corruption) that cannot wait for the normal staging-soak period:

1. The fix still passes through **every blocking CI stage** from `18_testing_strategy.md` Section 10 — no stage is ever skipped, even in an emergency (a core, non-negotiable rule, since skipping tests under pressure is precisely when a second, compounding incident is most likely).
2. The staging **deploy** step is still performed, but the staging **soak/observation period** that would normally precede a production promotion is compressed to a minimum smoke-test-only check, with an explicit, named incident-commander approval replacing the normal manual-approval gate's typical reviewer.
3. The change and its expedited path are documented in the post-incident review (Section 10) regardless of outcome — an emergency patch is never exempt from retrospective scrutiny just because it was urgent.

---

## 10. Production Readiness Checklist (Consolidated Gate)

Before the platform's initial production launch (and as a template for subsequent major-feature launches), the following must be explicitly verified, consolidating gates established throughout the series:

- [ ] Every blocking CI stage (`18_testing_strategy.md` §10) green on the release candidate.
- [ ] Full metric catalog and dashboard catalog (`17_observability.md` §4, §7) implemented and populated with real data from a staging soak period.
- [ ] GDPR purge-cascade dry-run (`18_testing_strategy.md` §19) completed and observed, not merely automated-tested.
- [x] KMS failure-domain entry added to `06_architecture.md` §12's table and this document's Section 8 (addressed in this release, per `16_security.md` §19's explicit action item).
- [ ] WhatsApp message templates (`15_notification_system.md` §4.3) submitted and approved for every `notification_type` the launch scope requires.
- [ ] Known-sender template library (`13_email_parser.md`) covers the MVP scope's curated sender list (`02_product_requirement_document.md` §6.1), each passing its sample-set validation.
- [ ] AI evaluation-set baseline (`18_testing_strategy.md` §8) established for the launch prompt version, with documented accuracy/calibration metrics as the reference point for future regression comparison.
- [ ] Secrets rotation schedule (`16_security.md` §4.4, §10) calendared, not merely documented as a policy.
- [ ] Gmail API and WhatsApp Business API quota/tier headroom verified against projected launch-scale usage (`23_scalability.md` cross-reference).
- [ ] On-call rotation and this document's Section 8 runbooks reviewed and acknowledged by the on-call team, not merely published.

---

## 11. Detailed Explanation: Why Production Deploys Require Manual Approval While Staging Does Not

A fully continuous-deployment pipeline (auto-promoting every green build straight to production) is a common, often desirable pattern for many SaaS products. This platform explicitly does **not** adopt that pattern for the production stage, while still keeping staging fully automatic, because:

1. **Asymmetric consequence profile:** A bad staging deploy affects no real user and no real financial data — it's a cheap, fast-iterating environment by design. A bad production deploy directly risks the correctness of a financial ledger (`04_domain_model.md`'s entire invariant structure exists specifically to prevent ledger corruption) and real user trust — the same "never simplify" philosophy that justified extensive upfront design documents also justifies a deliberate human checkpoint at the one place code meets real financial consequences.
2. **The manual approval gate is cheap relative to its benefit:** given the already-extensive automated gating (`18_testing_strategy.md` Section 10's full blocking-stage list), the marginal cost of one additional human click is small, while the benefit — a final human sanity-check, informed by context the automated pipeline cannot have (e.g., "we're mid-incident on an unrelated system right now, let's hold this deploy") — is disproportionately valuable for a system at this risk tier.
3. **This mirrors a pattern already established elsewhere in the series:** `16_security.md` Section 6.3 requires individual, audited `ops`-role provisioning rather than automatic/blanket grants; `13_email_parser.md` Section 6.3 requires regression-gated, not automatic, template publishing. A manual production-deploy approval gate is the same risk-proportionate-friction philosophy applied to the release process itself.

---

## 12. Component Responsibilities

| Component | Responsibility |
|---|---|
| Podman multi-stage build | Produce minimal, immutable, traceable runtime images |
| GitLab CI pipeline | Enforce the full test-gating chain and orchestrate staged promotion |
| Traefik | Edge TLS termination, routing, and rate-limiting enforcement (per `16_security.md` §5.3) |
| Goose migration step | Apply backward-compatible schema changes ahead of code rollout |
| On-call engineer + runbooks (Section 8) | Respond to failure-domain incidents with pre-documented, consistent procedure |
| Production-readiness checklist (Section 10) | The final, consolidated go/no-go gate for launch |

---

## 13. Trade-offs

| Decision | Trade-off |
|---|---|
| Manual production-approval gate (not full continuous deployment) | Slightly slower production release cadence than a fully automated pipeline would allow, in exchange for a deliberate, risk-proportionate human checkpoint before changes reach the financial ledger. |
| "Roll back code, not schema" as the default rollback path | Requires migration-authoring discipline (backward-compatibility, Section 6) that a simpler "always provide a down migration" approach would not, in exchange for a rollback path that's fast, well-tested, and low-risk in the common case. |
| Emergency patch procedure that still runs full CI (Section 9) | Slightly slower even in a genuine emergency than a "break glass, skip everything" option would be, in exchange for never compounding one incident with a second, process-skip-caused one. |

## 14. Advantages
- Every operational runbook (Section 8) is directly derived from and traceable to a failure domain or alert already specified in `06_architecture.md` and `17_observability.md`, meaning on-call engineers are never improvising a response to a scenario the design series hadn't already anticipated.
- The production-readiness checklist (Section 10) functions as a genuine, concrete closing of every "must be verified before launch" note scattered throughout the prior eighteen documents, rather than leaving launch-readiness as a vague sense of "I think we're done."
- The deliberate, risk-proportionate friction (manual approval, regression-gated template/prompt publishing, individual audited admin provisioning) is applied consistently across the entire series, not just at the deployment layer — this document's approval gate is recognizably the same design philosophy as several earlier ones, not a new, inconsistent rule.

## 15. Disadvantages
- The strict dev→staging→production promotion order (Section 3), even with the emergency-patch exception, does mean genuinely time-critical fixes take somewhat longer to reach production than an unconstrained hotfix process would allow — an accepted, deliberate cost given the financial-data stakes.
- Maintaining backward-compatible migration discipline (Section 6) indefinitely is an ongoing engineering tax on every schema change, requiring sustained team discipline rather than a one-time setup cost.

## 16. Future Improvements
- As the platform matures past MVP and the team builds confidence in its test suite's actual defect-catching power (measurable via `18_testing_strategy.md`'s Future Improvements on mutation testing), revisit whether the production manual-approval gate could be relaxed for specific, lower-risk categories of change (e.g., a documentation-only or dashboard-only change) without compromising the risk-proportionate philosophy for ledger-affecting changes.
- Introduce canary/percentage-based rollout (routing a small percentage of production traffic to a new version before full promotion) once traffic volume is high enough to make canary analysis statistically meaningful — currently likely premature at MVP scale.
- Build automated, scheduled (not just pre-launch) runbook-validation drills (deliberately inducing a controlled failure in staging to verify the runbook still matches reality) as the team's operational maturity grows.

## 17. Best Practices
- Every new failure domain or critical alert added to `06_architecture.md` Section 12 or `17_observability.md` Section 8 must have a corresponding runbook entry added to this document's Section 8 in the same change — these three sections across three documents must never drift out of sync.
- Every migration PR must be explicitly reviewed against Section 6's backward-compatibility rule as a standing, named checklist item in `20_coding_convention.md`'s review process — "does this work if the old code is still running against the new schema for a few minutes" must be asked and answered for every migration, not assumed.
- Rollback drills (actually performing a rollback in staging, not merely trusting the mechanism exists) should be exercised periodically, not only discovered to work or not work during a real incident.

## 18. Production Considerations
- The production-readiness checklist (Section 10) should be treated as a living document, re-run in full (not just for net-new items) before any subsequent major architectural change (e.g., if a module is ever extracted into a standalone service per `06_architecture.md` Section 18), not only at initial launch.
- The emergency-patch procedure (Section 9) should itself be drilled at least once before it's ever needed for a real incident, so the team isn't learning the expedited-approval process for the first time under pressure.
- KMS availability (flagged as a gap in `16_security.md` Section 19 and reiterated in Section 8's runbook table here) should be the single highest-priority item addressed before this document's production-readiness checklist can be honestly marked complete, given it was explicitly identified as missing from the original failure-domain table.

---

**Next Document:** `20_coding_convention.md` — specifies the platform's Go coding conventions: naming, formatting, error-handling idioms, the PR review checklist consolidating every "must be reviewed/enforced" note from prior documents (module-boundary imports, migration backward-compatibility, ownership-parameter inclusion, and more), and linter configuration.
