# Risk Analysis
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** All prior documents (01–21) — this document is a deliberate consolidation
**Feeds Into:** `23_scalability.md`, `24_future_roadmap.md`

---

## 1. Purpose

This document consolidates every risk surfaced across the preceding twenty-one documents — vendor dependency risk from `01_product_vision.md`, the eight event-storming hotspots, AI calibration/cost risk, WhatsApp quality-rating risk, GDPR compliance risk, and more — into a single risk register with explicit likelihood/impact assessment, mitigation status, and ownership.

No risk in this document is newly invented here; every row cites the document where it was first identified and the document where its mitigation is specified. This is a deliberate design choice: a risk register that duplicates mitigation detail inline would drift out of sync with the authoritative source; a risk register that only lists and scores risks, pointing precisely back to where each is handled, stays accurate by construction as those source documents evolve.

## 2. Scope

### 2.1 In Scope
- A complete risk register: every risk identified across the series, scored by likelihood × impact, with mitigation ownership and status.
- Risk categorization (technical, vendor/dependency, compliance/legal, product/business, operational).
- The risk-review cadence and escalation process.

### 2.2 Out of Scope
- Re-deriving mitigation detail already fully specified elsewhere (this document references, never duplicates).
- Financial/insurance risk-transfer mechanisms (a business/legal function outside engineering's direct scope).

---

## 3. Architecture: Risk Scoring Model

Each risk is scored on two axes, each 1 (Low) to 3 (High), multiplied for a composite score (1–9):

- **Likelihood:** How probable is this risk materializing under normal operation, absent further mitigation beyond what's already specified?
- **Impact:** If it materializes, how severe is the consequence — ranging from a minor, recoverable degradation to a compliance/legal or ledger-correctness incident?

| Composite Score | Risk Tier |
|---|---|
| 1–2 | Low — monitored, no immediate action required beyond existing mitigation |
| 3–4 | Medium — mitigation must be verified working, revisited periodically |
| 6 | High — requires active, ongoing attention; a candidate for production-readiness gating |
| 9 | Critical — must be fully mitigated and verified before launch; zero tolerance for an unmitigated gap |

---

## 4. Risk Register: Technical / Architectural

| ID | Risk | Likelihood | Impact | Score | Tier | Mitigation (Source) | Status |
|---|---|---|---|---|---|---|---|
| R-T1 | Duplicate ledger entries from at-least-once delivery | 3 | 3 | 9 | Critical | Outbox/Inbox pattern + INV-3 unique constraint (`06_architecture.md` §6, `08_database_design.md` §7.1) | Designed; verified by mandatory integration test (`18_testing_strategy.md` §6.2, Diagram 5) |
| R-T2 | Silent ingestion gap from missed Gmail Watch renewal + missed push (H2) | 2 | 3 | 6 | High | Dual-trigger push+poll design (`06_architecture.md` §6.6, `12_gmail_connector.md` §5.3) | Designed; poll-fallback liveness itself monitored (`17_observability.md` §8) |
| R-T3 | AI extraction fallback storm on template breakage (H3) | 2 | 2 | 4 | Medium | Rate-limited queue concurrency + failure-rate-spike alerting (`06_architecture.md` §6.5, `13_email_parser.md` §8) | Designed; threshold tuning pending real traffic data |
| R-T4 | Analytics read-model staleness misleading users (H6) | 2 | 2 | 4 | Medium | Projection lag metric + `projectionFreshness` API transparency (`06_architecture.md` §6.7, `09_api_specification.md` §6.3) | Designed; freshness SLO target still TBD pending production data (`17_observability.md` §9) |
| R-T5 | Cross-module boundary erosion over time as team grows | 2 | 2 | 4 | Medium | CI-enforced import linter + schema-permission isolation (`06_architecture.md` §5.3) | Designed; requires periodic linter-configuration audit as module map evolves |
| R-T6 | Outbox Relay poll latency/database load trade-off mistuned at scale | 2 | 2 | 4 | Medium | Tunable poll interval/batch size, explicitly flagged for load-testing (`06_architecture.md` §20, §23_scalability.md) | Pending load-test data |
| R-T7 | `financial_events` polymorphic `jsonb` payload becoming an unstructured "junk drawer" | 2 | 2 | 4 | Medium | Per-type documented JSON schema + application-layer validation discipline (`04_domain_model.md` §18, `08_database_design.md` §18) | Requires ongoing review discipline, not a one-time fix |

---

## 5. Risk Register: Vendor / External Dependency

| ID | Risk | Likelihood | Impact | Score | Tier | Mitigation (Source) | Status |
|---|---|---|---|---|---|---|---|
| R-V1 | Gmail API quota exhaustion as user base scales | 2 | 3 | 6 | High | Quota-consumption monitoring + proactive increase-request process (`12_gmail_connector.md` §8, `23_scalability.md`) | Requires calendared capacity planning, not a one-time grant |
| R-V2 | WhatsApp Business API messaging-tier ceiling or quality-rating degradation (H7-adjacent) | 2 | 3 | 6 | High | Tier/block-rate monitoring, `UTILITY`-category templates, cross-functional quality review cadence (`15_notification_system.md` §9) | Requires standing product+engineering review cadence |
| R-V3 | LLM provider outage or pricing/availability change | 2 | 2 | 4 | Medium | Provider-agnostic client abstraction (OpenAI-compatible shape) (`11_ai_blueprint.md` §8) | Designed; a provider swap is a config change, not a code change |
| R-V4 | WhatsApp template rejection or approval-lead-time blocking a feature launch (H7) | 2 | 2 | 4 | Medium | Explicit lead-time dependency documented in feature-planning process (`15_notification_system.md` §4.3) | Requires feature-planning discipline, not purely an engineering fix |
| R-V5 | Gmail `historyId` retention window behaving differently than assumed | 1 | 2 | 2 | Low | Bounded fallback re-sync, not a hardcoded assumption (`12_gmail_connector.md` §6.2) | Designed; monitored via `HistorySyncFallbackTriggered` frequency |
| R-V6 | KMS provider outage blocking token refresh platform-wide | 2 | 3 | 6 | High | Identified as a gap in the original failure-domain table; explicitly flagged for correction (`16_security.md` §4.3, §19, `19_deployment.md` §8) | **Open — must be closed before launch sign-off** |

---

## 6. Risk Register: Compliance / Legal

| ID | Risk | Likelihood | Impact | Score | Tier | Mitigation (Source) | Status |
|---|---|---|---|---|---|---|---|
| R-C1 | Partial GDPR erasure cascade leaving residual personal data (H8) | 1 | 3 | 3→ treated as Critical given impact floor | Critical | Explicit multi-module cascade with audit-exempt logging; redundant integration+E2E testing (`07_sequence_diagrams.md` Diagram 8, `18_testing_strategy.md` §12) | Designed; requires staging dry-run sign-off before launch (`19_deployment.md` §10) |
| R-C2 | LLM provider exposure of PII beyond minimization controls | 2 | 3 | 6 | High | Masking/stripping rules, classification tiers (`11_ai_blueprint.md` §9, `16_security.md` §7) | Designed; requires fixture-based unit testing per `11_ai_blueprint.md` §18 |
| R-C3 | Erasure request exceeding regulatory response window under load | 1 | 3 | 3 (impact-floored as High given legal exposure) | High | Live SLO monitoring with Critical alerting (`17_observability.md` §8, §9) | Designed; depends on R-T1/R-T6 mitigations holding under real load |
| R-C4 | Global category-mapping aggregation inadvertently leaking individual user behavior | 1 | 2 | 2 | Low | Privacy-preserving aggregation design, reviewed against `16_security.md` whenever methodology changes (`11_ai_blueprint.md` §7, §19) | Requires re-review on any methodology change — standing obligation |

**Note on impact-floored scoring (R-C1, R-C3):** Composite scoring in Section 3 is a guideline, not a mechanical override — per `16_security.md` Section 9.2 and `17_observability.md` Section 9's closing note, compliance-critical risks with a low numeric likelihood but severe impact are **never** down-tiered to "Medium" purely by the multiplication formula; this register explicitly floors them at the tier their impact alone warrants, consistent with those documents' explicit rejection of treating a single compliance miss as an ordinary, budget-tolerable event.

---

## 7. Risk Register: Product / Business

| ID | Risk | Likelihood | Impact | Score | Tier | Mitigation (Source) | Status |
|---|---|---|---|---|---|---|---|
| R-P1 | Confidence threshold miscalibrated at launch, causing excessive review-queue burden or excessive wrong auto-finalizations | 3 | 2 | 6 | High | Explicit directive to revisit using real data within first operational month (`02_product_requirement_document.md` §12, `11_ai_blueprint.md` §6.3) | Requires scheduled post-launch review, not a launch-day-only decision |
| R-P2 | Known-sender template coverage insufficient at launch, pushing excessive volume to (slower, costlier) AI path | 2 | 2 | 4 | Medium | Curated MVP sender list + continuous template-library expansion informed by Parser Monitoring (`02_product_requirement_document.md` §6.1, `13_email_parser.md` §15) | Ongoing content/ops responsibility, not a one-time build |
| R-P3 | Notification fatigue/low relevance degrading WhatsApp quality rating (overlaps R-V2) | 2 | 2 | 4 | Medium | Cross-functional quality-rating review cadence (`15_notification_system.md` §17) | Requires product+engineering shared ownership |
| R-P4 | AI evaluation-set not representative of real-world long-tail senders | 2 | 2 | 4 | Medium | Evaluation set explicitly grows from production-diagnosed failures, not frozen at launch (`18_testing_strategy.md` §8.1) | Ongoing, self-correcting by design — requires the operational habit of actually adding fixtures |

---

## 8. Risk Register: Operational

| ID | Risk | Likelihood | Impact | Score | Tier | Mitigation (Source) | Status |
|---|---|---|---|---|---|---|---|
| R-O1 | Alert fatigue causing real incidents to be missed among noise | 2 | 2 | 4 | Medium | Alert-to-actioned-ratio tracked as its own metric, reviewed within first operational month (`17_observability.md` §17) | Requires scheduled review |
| R-O2 | n8n scope creep into core financial logic over time | 1 | 3 | 3 (floored to High given correctness-guarantee bypass severity) | High | Explicit governance checklist + narrowly-scoped, write-forbidden credential (`14_workflow_engine.md` §5, §6) | Designed; requires periodic compliance audit of actual workflow definitions (`14_workflow_engine.md` §13) |
| R-O3 | CI pipeline flakiness eroding team discipline to actually heed its gates | 2 | 2 | 4 | Medium | Pipeline health monitored with same rigor as the application (`18_testing_strategy.md` §19) | Ongoing operational discipline |
| R-O4 | Monitor-the-monitor gaps (poll-fallback liveness, Outbox relay liveness, aggregation-job liveness all failing silently) | 2 | 3 | 6 | High | Explicit liveness metrics for each safety-net component (`12_gmail_connector.md` §17, `13_email_parser.md` §19) | Designed; requires the full safety-net-of-safety-nets to be verified as actually wired in production, not merely documented |
| R-O5 | Credential/secret rotation treated as incident-reactive rather than calendared | 2 | 2 | 4 | Medium | Explicit calendared rotation requirement (`16_security.md` §18) | Requires an actual operational calendar entry, not just a written policy |

---

## 9. Consolidated Risk Heat Map (By Tier)

| Tier | Risk IDs |
|---|---|
| **Critical** | R-T1, R-C1 |
| **High** | R-T2, R-V1, R-V2, R-V6, R-C2, R-C3, R-P1, R-O2, R-O4 |
| **Medium** | R-T3, R-T4, R-T5, R-T6, R-T7, R-V3, R-V4, R-P2, R-P3, R-P4, R-O1, R-O3, R-O5 |
| **Low** | R-V5, R-C4 |

**Single open item requiring action before launch sign-off:** R-V6 (KMS failure-domain gap) is the only risk in this register whose mitigation is marked **Open** rather than "Designed" — every other row has a specified, documented mitigation already designed into the system; R-V6 is explicitly flagged across three prior documents (`16_security.md`, `19_deployment.md`) as needing to actually be added to the failure-domain table and the KMS-outage runbook, and this register makes that the one unambiguous, must-close-before-launch action item.

---

## 10. Risk Review Cadence & Escalation

| Cadence | Activity |
|---|---|
| **Weekly** (engineering standup-adjacent) | Review any newly-triggered High/Critical alert from `17_observability.md` against this register — does it correspond to a known risk behaving as expected, or a new, previously-unidentified risk requiring a new register entry? |
| **Monthly** | Review R-P1 (confidence calibration), R-O1 (alert fatigue), per their explicit "revisit using real data" obligations |
| **Quarterly** | Full register review: re-score every risk given actual production experience; verify no Medium-tier risk has silently drifted toward High without the register being updated |
| **Ad hoc, mandatory** | Any new feature, module, or external dependency addition must be checked against this register for whether it introduces a new risk category, following the same scoring model (Section 3) |

**Escalation:** Any risk whose likelihood or impact is reassessed upward into the Critical tier during a review triggers the same escalation path as a production incident (`19_deployment.md` Section 8's runbook-style severity routing) — a risk crossing into Critical is treated with the same urgency as an active incident, not merely noted for a future planning cycle.

---

## 11. Detailed Explanation: Why This Register Cites Rather Than Duplicates Mitigation Detail

A natural alternative design would have this document restate each mitigation's full mechanism inline, producing a single, self-contained risk document. This document deliberately does **not** do that, for a reason directly analogous to the database-schema-ownership philosophy established in `06_architecture.md` and `08_database_design.md`: **a single source of truth per concern, referenced rather than copied.**

1. If R-T1's mitigation detail (the Outbox/Inbox mechanism) were restated here in full, any future refinement to that mechanism in `06_architecture.md` would require a synchronized edit to this document too, or the two would silently drift — exactly the "documentation and code must not drift" discipline established in `07_sequence_diagrams.md` Section 17 and `10_backend_blueprint.md` Section 19, applied here to documentation-to-documentation consistency rather than documentation-to-code.
2. This register's actual value-add is **not** mitigation detail (which belongs, in full, exactly once, in its originating document) — it is the **aggregation, scoring, and cross-category view** that no single prior document could provide on its own, since each prior document reasonably focused on its own slice (architecture, security, AI, deployment) without needing to weigh its risks against every other document's risks on a common scale. That comparative, prioritized view is this document's unique contribution, and is exactly what a reviewer, a new engineering lead, or a compliance auditor most needs when deciding where to focus limited attention.

---

## 12. Component Responsibilities

| Component | Responsibility |
|---|---|
| This risk register | Aggregation, consistent scoring, and prioritized visibility across every risk category |
| Each originating document (cited per row) | The authoritative, complete mitigation specification — never duplicated here |
| `17_observability.md`'s alerting | The live, operational signal that a registered risk is materializing in production |
| Quarterly review process (Section 10) | The mechanism preventing this register from becoming a stale, point-in-time snapshot |

---

## 13. Trade-offs

| Decision | Trade-off |
|---|---|
| Citation-only register (no duplicated mitigation detail) | Requires a reader to follow references to understand *how* a risk is mitigated, in exchange for a register that stays accurate without a parallel-maintenance burden. |
| Impact-floored scoring overriding the pure multiplication formula for R-C1/R-C3/R-O2 | Introduces a small amount of scoring-model subjectivity/judgment, in exchange for never allowing a mechanically-low composite score to mask a genuinely severe compliance or correctness exposure. |
| Explicit single "must-close-before-launch" callout (R-V6) rather than treating the whole register as equally launch-blocking | Requires confidence that every other row's "Designed" status is genuinely trustworthy, in exchange for giving the team one unambiguous, prioritized action item rather than an undifferentiated list of 25+ rows to triage under launch pressure. |

## 14. Advantages
- This register gives any reader — engineering lead, new hire, auditor, or investor-facing technical reviewer — a single, scannable, honestly-scored view of the platform's complete risk posture, something no single one of the prior twenty-one documents could offer alone.
- The explicit citation discipline (Section 11) means this document will age gracefully rather than becoming a stale, drifted artifact the moment any underlying mitigation is refined.
- The impact-floor scoring convention (Section 6, Section 8) correctly resists the common risk-register failure mode of letting a low-probability, catastrophic-impact risk get mathematically laundered into an apparently-tolerable "Medium" score.

## 15. Disadvantages
- A register this granular (25 distinct risk rows) still requires active maintenance discipline (Section 10's cadence) to remain trustworthy — an unmaintained risk register is arguably worse than none at all, since it creates false confidence that risks are being tracked.
- The citation-heavy structure, while avoiding drift, does mean this document alone is insufficient for someone who needs to actually *act* on a mitigation — they must always follow through to the source document, a minor but real navigation cost for urgent, in-the-moment incident response (mitigated by `19_deployment.md`'s runbooks being the faster, incident-specific reference for that exact situation).

## 16. Future Improvements
- Once the platform has sufficient production operating history, replace several of this register's likelihood estimates (currently engineering judgment calls at design time) with actual observed-frequency data from `17_observability.md`'s metrics — e.g., R-T3's likelihood should be recalibrated using real observed template-failure-rate-spike frequency once available.
- Consider a lightweight, periodically-regenerated "risk register freshness check" (a script verifying every cited section reference still exists and matches its described content) as the document series' own line count grows, to catch citation drift mechanically rather than relying solely on the quarterly manual review.
- Expand the register with explicit financial/business-continuity risk categories (e.g., cloud-provider regional outage, company-level business continuity) if/when the platform's risk-management scope is formally expanded beyond pure engineering risk, likely in coordination with a broader organizational risk function.

## 17. Best Practices
- No new document in this series (or any future addition to it) should introduce a previously-unregistered significant risk without that risk being added to this register in the same change — this document's completeness depends on that discipline being maintained going forward, not just at its initial authoring.
- The "must-close-before-launch" callout mechanism (Section 9) should be used sparingly and precisely — if every row claimed launch-blocking urgency, the signal would be lost; reserving it for genuinely outstanding, unaddressed gaps (as with R-V6 currently) keeps it meaningful.
- Risk re-scoring during the quarterly review (Section 10) should be justified with a brief rationale (why did the likelihood or impact change), not merely a number update — preserving an audit trail of the register's own evolution, mirroring the audit-trail philosophy applied throughout the rest of the platform's design.

## 18. Production Considerations
- R-V6 (KMS failure-domain gap) must be explicitly resolved — added to `06_architecture.md` Section 12's table, with a corresponding runbook entry in `19_deployment.md` Section 8 — before this register can honestly report zero open launch-blocking items; this is the concrete, final action this document surfaces for immediate engineering attention.
- The quarterly review cadence (Section 10) should be a calendared, assigned-owner activity from day one of production operation, not an informal "we should probably do this sometime" intention — risk registers that lack an assigned owner and cadence reliably decay into unmaintained artifacts within a few months of real operational pressure.

---

**Next Document:** `23_scalability.md` — specifies the platform's scaling strategy in full: the specific thresholds at which `financial_events` partitioning (`08_database_design.md` §7.1), Outbox Relay tuning (`06_architecture.md` §20, R-T6), and module-extraction-to-microservice (`06_architecture.md` §18, starting with `understanding`) should be activated, plus capacity-planning guidance for Gmail/WhatsApp quota headroom (R-V1, R-V2).
