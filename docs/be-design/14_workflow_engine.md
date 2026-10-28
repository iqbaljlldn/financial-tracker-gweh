# Workflow Engine
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `06_architecture.md`, `10_backend_blueprint.md`, `13_email_parser.md`
**Feeds Into:** `15_notification_system.md`, `19_deployment.md`, `22_risk_analysis.md`

---

## 1. Purpose

This document specifies **n8n's precisely-bounded role** within the platform. `06_architecture.md` Section 11 already establishes the load-bearing constraint that n8n must never process core financial data — this document exists to make that boundary **operational and unambiguous**: exactly which automations n8n owns, exactly how it integrates with the core Go backend, and exactly what a reviewer should reject if a future workflow proposal tries to cross the line.

Without an explicit document like this, "workflow automation" tooling has a well-known failure mode in growing platforms: it starts with a legitimate, low-stakes use case (an ops alert), and — because it's easy and visual — gradually accretes more and more business logic until a critical process depends on a workflow that has none of the correctness guarantees (Outbox/Inbox, transactional integrity, code review, automated testing) that `06_architecture.md` designed into the core system. This document is the guardrail against that drift.

## 2. Scope

### 2.1 In Scope
- The precise definition of n8n's sanctioned responsibility surface.
- The explicit, non-negotiable exclusion list (what n8n must never do).
- Integration boundary: how n8n workflows are triggered by and communicate with the core platform.
- Governance process for proposing and reviewing new n8n workflows.
- Example sanctioned workflow specifications.

### 2.2 Out of Scope
- n8n's own internal architecture/deployment (a standard, off-the-shelf tool — general deployment detail in `19_deployment.md`).
- The core Asynq-based event-processing engine (the actual "workflow engine" for financial processing, despite this document's title referring to n8n specifically) → `06_architecture.md` Section 6.

---

## 3. Architecture: n8n's Position Relative to the Core Platform

```
┌──────────────────────────────────────────────────────────────┐
│                     CORE PLATFORM (Go binary)                    │
│   Identity │ Ingestion │ Understanding │ FinancialCore │ Budget  │
│   Analytics │ Engagement │ Administration                        │
│                                                                    │
│   [ALL financial processing, extraction, ledger writes, budget    │
│    evaluation, and user-facing notification composition/delivery  │
│    happen exclusively here, via Outbox/Inbox-guaranteed paths]     │
└───────────────────────────┬────────────────────────────────────┘
                              │
                    (narrow, one-directional
                     integration surface — Section 5)
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                            n8n                                    │
│                                                                    │
│  Sanctioned:                                                       │
│   - Ops/on-call alerting workflows (Slack/PagerDuty routing)         │
│   - Template-maintenance-assistance workflows (Section 6.1)           │
│   - Ad hoc administrative reporting/export automations                │
│   - Vendor-webhook-to-internal-alert bridging (non-financial)          │
│                                                                    │
│  Forbidden (absolute):                                              │
│   - Any FinancialEvent creation, modification, or deletion            │
│   - Any confidence gating or ledger finalization logic                 │
│   - Any budget evaluation or alert-firing logic                        │
│   - Any user-facing WhatsApp summary/report composition                 │
│   - Any OAuth token handling                                            │
└──────────────────────────────────────────────────────────────┘
```

---

## 4. Sanctioned Responsibility Surface

n8n owns automations that are **operational, administrative, or ops-facing** — never customer-facing financial processing. Concretely:

### 4.1 Operational Alerting & Routing
- Bridging Prometheus/Grafana alerts (`17_observability.md`) into Slack/PagerDuty/email with enriched, human-readable context (e.g., formatting a Parser Monitoring failure-rate-spike alert from `13_email_parser.md` Section 8.2 into a nicely-formatted Slack message with links) — the alert *condition* is still computed and fired by the core platform's Prometheus rules; n8n only handles *routing and presentation*.
- Escalation workflows (e.g., "if a `dead-letter` queue alert is unacknowledged in Slack for 30 minutes, page the secondary on-call") — pure notification-routing logic, no financial data involved.

### 4.2 Template-Maintenance-Assistance Workflows
- Per `13_email_parser.md` Section 14's noted future improvement: a workflow that, when a Parser Monitoring failure-rate alert fires, automatically gathers the relevant failing sample messages (via a narrow, read-only internal API call — Section 5) and posts a formatted "template needs attention" ticket/thread with the diagnostic context, **reducing the manual diagnostic legwork for the template author** without n8n itself authoring, validating, or publishing the template (that remains a core-platform, code-reviewed, regression-gated action per `13_email_parser.md` Section 6.3).

### 4.3 Ad Hoc Administrative Reporting
- Scheduled workflows that call **read-only** Administration API endpoints (`09_api_specification.md` Section 6.6) to generate periodic internal reports (e.g., a weekly "connector health summary" emailed to the engineering team) — strictly consuming already-computed, already-correct data from the core platform, never computing anything itself.

### 4.4 Non-Financial Vendor Webhook Bridging
- If a future non-financial third-party tool (e.g., a customer support ticketing system, a status-page provider) needs simple webhook-triggered internal notifications, n8n is an appropriate, low-friction place for that glue logic — explicitly because it is unrelated to the financial data pipeline this document is protecting.

---

## 5. Integration Boundary

### 5.1 One-Directional, API-Mediated Access

n8n workflows interact with the core platform **exclusively** through:
1. **Inbound:** Webhook triggers from the core platform's own alerting/observability stack (Prometheus Alertmanager configured to call an n8n webhook URL) — the core platform initiates, n8n reacts.
2. **Outbound (read-only):** n8n workflows may call a small, explicitly-enumerated set of **read-only, ops-scoped** internal API endpoints (the `/admin/*` surface from `09_api_specification.md` Section 6.6, using a dedicated, narrowly-scoped `role: "ops"` service credential distinct from any human operator's credential) to fetch diagnostic/reporting data.

**n8n never calls any write-capable endpoint, and never has direct database access to any module's schema.** This is enforced both by **not provisioning** n8n's service credential with any write-scoped permission (Section 5.2) and by **not granting n8n any network path to PostgreSQL/Redis directly** (network-policy-level enforcement, detailed in `19_deployment.md`) — a defense-in-depth pairing of application-level and infrastructure-level controls, mirroring the same philosophy applied to module-boundary enforcement in `06_architecture.md` Section 5.3.

### 5.2 Credential Scoping

The n8n service account/API credential is provisioned with a role that has access to **only** the specific read-only Administration endpoints enumerated in Section 4.3/4.2 — not a blanket `role: "ops"` grant equivalent to a human operator's. This is a distinct, more restrictive credential tier, explicitly designed so that even a fully compromised n8n instance (a real risk category for any workflow-automation tool with broad webhook/integration surface) cannot read or write core financial data, cannot access user PII beyond what's already aggregated into the specific reporting endpoints it's scoped to, and structurally cannot touch the ledger.

---

## 6. Governance Process for New Workflows

Any new n8n workflow proposal must pass this checklist before implementation:

1. **Does it create, modify, or delete any `FinancialEvent`, `Budget`, `CategoryAssignment`, or `NotificationJob` record, even indirectly?** If yes → **rejected**, must be implemented as a core-platform Command Handler (`10_backend_blueprint.md` Section 4) instead.
2. **Does it compute anything that affects confidence gating, budget alert firing, or notification content shown to an end user?** If yes → **rejected**, same reasoning as above — this logic must live where it has Outbox/Inbox transactional guarantees and code-review/test coverage.
3. **Does it require any credential beyond the narrow, enumerated read-only Administration scope (Section 5.2)?** If yes → requires explicit Security review and a credential-scope amendment to this document before proceeding, not an ad hoc broader grant.
4. **Is it operational/administrative/ops-facing, strictly downstream of already-correct core-platform data or events?** If yes to this and no to 1–3 → **sanctioned**, may proceed via normal workflow-authoring process (no code review needed in the traditional sense, but the workflow definition itself should still be version-controlled — n8n supports workflow-as-JSON export — and peer-reviewed for the checklist above).

This checklist must be attached to any PR/change request that introduces or materially modifies an n8n workflow, and Section 4's list must be updated whenever a new category of sanctioned workflow is approved, keeping this document the living source of truth for the boundary — mirroring the same "documentation and code must not drift" discipline established in `07_sequence_diagrams.md` Section 17 and `10_backend_blueprint.md` Section 19.

---

## 7. Detailed Explanation: Why This Boundary Is Load-Bearing, Not Merely Tidy

It would be easy to treat this document as a style preference — "we use n8n for glue code because it's convenient, and Go for the real work because it's more robust." The actual justification is stronger and more specific:

1. **The entire correctness argument of `06_architecture.md`** — exactly-once effect via Outbox/Inbox, module-boundary CI enforcement, invariant enforcement at the domain layer — applies **only** to code paths that go through the core platform's Command Handlers and transactional persistence. An n8n workflow that, for example, directly called a write API to "helpfully" auto-correct a low-confidence event based on some visual-workflow logic would completely bypass `04_domain_model.md`'s `EventOverride` audit trail, `06_architecture.md`'s Inbox idempotency guard, and `18_testing_strategy.md`'s test coverage — reintroducing exactly the class of financial-data-integrity risk this entire document series exists to prevent.
2. **n8n workflows are visual/low-code by design**, which is precisely their value for ops automation (fast to build, easy for non-Go-specialist team members to maintain) — but this same property means they are **not naturally subject to the same rigor** (unit tests, PR review with domain expertise, CI boundary linting) as the Go codebase. Keeping n8n confined to operationally-reversible, non-financial-data-touching automations means a workflow authoring mistake has a bounded, recoverable blast radius (a missed Slack alert, a malformed internal report) rather than a ledger-corrupting one.
3. **This boundary is also what keeps the "modular monolith, microservice-ready" story (`06_architecture.md` Section 3.1) honest.** If core business logic quietly lived partly in n8n, no future service-extraction effort could reason about the system's true behavior from the Go codebase alone — the system's actual logic would be split across two fundamentally different tooling paradigms with different versioning, testing, and deployment models.

---

## 8. Component Responsibilities

| Component | Responsibility |
|---|---|
| n8n instance | Ops/administrative workflow execution, strictly within Section 4's sanctioned surface |
| Prometheus Alertmanager | Initiates webhook calls into n8n (one-directional trigger) |
| Core platform `/admin/*` read-only endpoints | The sole data-access surface n8n is permitted to consume |
| n8n service credential | Narrowly-scoped, read-only, distinct from human ops credentials |
| Governance checklist (Section 6) | The enforced gate preventing scope creep into core financial logic |

---

## 9. Trade-offs

| Decision | Trade-off |
|---|---|
| Strict n8n exclusion from all financial-data-touching logic | Some legitimately convenient "just wire it up visually" opportunities are deliberately forgone (e.g., a visual workflow *could* technically implement simple budget-alert routing), in exchange for preserving the correctness guarantees that are non-negotiable for a financial ledger. |
| Read-only-only, narrowly-scoped n8n credential | Slightly limits what ops automations are possible without a core-platform code change (e.g., n8n cannot itself trigger a re-sync or template republish, even for legitimate ops convenience) — an intentional friction, pushing any write-capable automation need through the reviewed, tested Command Handler path instead. |
| Governance checklist as a manual PR-attached process (not automated) | Relies on process discipline rather than a technical control for the *proposal* stage (though the *execution* stage is technically enforced via credential scoping and network policy) — accepted because the checklist itself is short and the technical enforcement (Section 5) is the actual hard backstop. |

## 10. Advantages
- The explicit, numbered checklist (Section 6) gives every engineer and reviewer an unambiguous, fast way to evaluate a workflow proposal, rather than relying on tribal knowledge about "what n8n is for here."
- The defense-in-depth credential/network scoping (Section 5) means the boundary holds even under a worst-case n8n compromise scenario, not merely under well-intentioned normal usage.
- Confining n8n to genuinely low-stakes, easily-reversible automations means the team gets n8n's real productivity benefit (fast ops tooling) without importing its correctness-rigor gap into the financial core.

## 11. Disadvantages
- Some ops convenience is sacrificed — a handful of borderline-useful automations (e.g., visually building a "one-click template republish" button) are explicitly disallowed even though they'd be easy to build in n8n, requiring the equivalent core-platform admin action to be built properly instead.
- Maintaining two distinct tooling paradigms (Go core platform + n8n ops layer) does mean two different skill sets/review processes for the engineering team to maintain competency in, even though their scopes are cleanly separated.

## 12. Future Improvements
- If n8n's read-only reporting usage (Section 4.3) grows substantially, consider whether a purpose-built internal admin dashboard (rather than n8n-generated reports) better serves the need — n8n is a pragmatic MVP-era choice for this, not necessarily the permanent one.
- Periodically audit actual n8n workflow definitions (via their version-controlled JSON exports, per Section 6) against this document's Section 4 sanctioned list, to catch any drift that may have crept in despite the governance checklist — a scheduled compliance review, not a one-time check.

## 13. Best Practices
- Every n8n workflow must be exported and version-controlled (JSON export committed to a dedicated ops-automation repository or directory) — no workflow should exist only in the n8n UI with no reviewable history.
- Any engineer proposing a new n8n workflow must self-apply the Section 6 checklist and state the answer explicitly in the proposal, not leave it to the reviewer to infer.
- The n8n service credential's actual granted scope should be periodically verified against Section 5.2's intended scope (a credential-drift audit), since manual provisioning processes are a common source of "temporarily" over-broad grants that are never revisited.

## 14. Production Considerations
- n8n's own availability is explicitly **not** on the critical path for any core financial processing (per this document's entire premise) — its outage runbook (`19_deployment.md`) should reflect this: an n8n outage degrades ops alerting convenience, never ledger correctness or user-facing notification delivery.
- Network-policy enforcement preventing n8n from reaching PostgreSQL/Redis directly (Section 5.1) must be verified as part of the deployment pipeline's security checks (`16_security.md`), not merely documented as an intended architecture.
- The narrowly-scoped n8n credential (Section 5.2) must be included in the platform's regular credential-rotation and access-review process, same as any other service credential, despite its restricted scope — restricted does not mean exempt from rotation discipline.

---

**Next Document:** `15_notification_system.md` — specifies the Engagement module in full: WhatsApp Business API integration mechanics, message template management (resolving Hotspot H7's pre-approval constraint), the constrained-intent reply-parsing design (resolving H4), and the two-tier delivery-confirmation model introduced in `07_sequence_diagrams.md` Diagram 7.
