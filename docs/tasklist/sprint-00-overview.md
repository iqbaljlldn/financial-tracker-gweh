# Sprint Overview & Index
## Financial Event Platform — Implementation Checklist Series

**Assumptions:** 2-week sprints, small full-stack team (1–3 backend, 1 frontend, shared DevOps), targeting MVP scope as defined in `02_product_requirement_document.md` §6.1. Adjust pacing to your actual team size — the sequence and dependencies matter more than the exact sprint count.

**Frontend stack assumption (not specified in the 24-document series, which was explicitly backend-only — React + Vite is assumed here as the closest match to typical usage; swap freely):** The frontend consumes `09_api_specification.md`'s contract exclusively — it never talks to the database, Gmail, WhatsApp, or the LLM provider directly. Frontend work is threaded through Sprints 1–9 **in parallel** with the backend module each sprint builds (its own "Frontend (Parallel Track)" section), since each screen depends on that sprint's API endpoints existing.

**Observability is not a one-sprint concern.** Every sprint from Sprint 2 onward has its own "Observability" checklist items (metrics/traces/logs for the capability just built that sprint) — Sprint 10 is reserved for closing any gaps, wiring the cross-cutting dashboards, and activating alerting, not for building observability from scratch.

**Source series:** Every checklist item below cites the design document/section it implements. If an item is unclear, that document is the authority — this checklist does not redefine anything, only sequences and tracks it.

---

## Sprint Map

| Sprint | Theme | Primary Documents |
|---|---|---|
| [Sprint 0](sprint-00-foundation.md) | Foundation & Tooling | `21_project_structure.md`, `20_coding_convention.md`, `19_deployment.md` |
| [Sprint 1](sprint-01-identity-db-foundation.md) | Identity & Access + Database Foundations | `08_database_design.md`, `16_security.md` §4–6 |
| [Sprint 2](sprint-02-ingestion-onboarding.md) | Ingestion I — OAuth Linking & Initial Sync | `12_gmail_connector.md` §4, §6.3 |
| [Sprint 3](sprint-03-ingestion-realtime.md) | Ingestion II — Watch, Incremental Sync, Poll Fallback | `12_gmail_connector.md` §5–9 |
| [Sprint 4](sprint-04-understanding-parser.md) | Understanding I — Deterministic Parser | `13_email_parser.md` |
| [Sprint 5](sprint-05-understanding-ai.md) | Understanding II — AI Extractor | `11_ai_blueprint.md` |
| [Sprint 6](sprint-06-financial-core.md) | Financial Core — Ledger | `04_domain_model.md` §5, `08_database_design.md` §7 |
| [Sprint 7](sprint-07-budget.md) | Budget Module | `04_domain_model.md` §9, `08_database_design.md` §10 |
| [Sprint 8](sprint-08-analytics.md) | Analytics Module (CQRS Read Side) | `06_architecture.md` §7, `08_database_design.md` §11 |
| [Sprint 9](sprint-09-engagement.md) | Engagement — WhatsApp Notifications | `15_notification_system.md` |
| [Sprint 10](sprint-10-security-observability.md) | Security & Observability Hardening | `16_security.md`, `17_observability.md` |
| [Sprint 11](sprint-11-testing-qa.md) | Testing & QA Hardening | `18_testing_strategy.md` |
| [Sprint 12](sprint-12-deployment-launch.md) | Deployment & Launch Readiness | `19_deployment.md`, `22_risk_analysis.md` |

---

## How to Use These Checklists

- Each sprint file lists **Goal**, **Entry criteria** (what must be true before starting), **Checklist items** (grouped by concern), and **Exit criteria** (what must be true/demoable before moving on).
- Check items off as completed; don't skip a sprint's exit criteria to start the next sprint early — most sprints have hard dependencies on the prior one (e.g., Sprint 6 needs Sprint 4+5's `CandidateFinancialEvent` output to exist).
- Items tagged **[GATE]** are hard blockers per the design series (e.g., CI test gates, regression-gated publishing) — these cannot be skipped or deferred.
- Items tagged **[DEFER-OK]** can slip to a later sprint without blocking progress, if time pressure demands it.

## Post-MVP

Once Sprint 12 closes, the single open action item from `22_risk_analysis.md` (R-V6, KMS failure-domain gap) must be resolved before calling the platform launch-ready. After that, see `24_future_roadmap.md` §4 for the mandatory post-launch calibration work (confidence threshold tuning, risk re-scoring, etc.) — that is effectively "Sprint 13" and is not optional.
