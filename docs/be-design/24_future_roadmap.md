# Future Roadmap
## Personal Finance Assistant Platform — Financial Event Platform

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** All prior documents (01–23) — this document is the series' closing synthesis
**Feeds Into:** N/A — this is the final document in the series; it feeds back into product/engineering planning cycles

---

## 1. Purpose

This is the final document in the twenty-four-document Financial Event Platform specification series. It consolidates three things that have been deliberately kept separate throughout the series, for exactly this moment of synthesis:

1. The Vision's Stage 1–4 maturity trajectory (`01_product_vision.md` Section 7).
2. The PRD's MVP / Phase 2 / Phase 3 functional scoping (`02_product_requirement_document.md` Section 6).
3. Every individual "Future Improvements" section scattered across all twenty-three prior documents — each written in the context of its own document, now gathered, cross-referenced, and sequenced into one coherent, prioritized forward view.

Where every other document in this series specified something to be **built now** (even if some capabilities are gated to Phase 2/3), this document specifies what comes **after** — the roadmap for evolving the platform once the MVP described in Documents 01–23 is live, instrumented, and generating real production data.

## 2. Scope

### 2.1 In Scope
- The full Stage 1–4 / Phase 1–3 trajectory, unified into one roadmap view.
- A consolidated, deduplicated catalog of every "Future Improvement" from Documents 01–23, organized by theme and sequenced by dependency.
- Prioritization rationale connecting roadmap items back to the Vision's north star (Stage 4, Advisory Intelligence).
- Explicit "what must be true before this is viable" dependency chains between roadmap items.

### 2.2 Out of Scope
- Re-specifying any future capability in the implementation-ready detail this series applied to MVP scope — future-phase items are deliberately left at a lighter specification depth here; when a given item is actually scheduled, it warrants its own focused design pass (potentially its own mini-series, following this document series' own methodology).
- Firm calendar dates or sprint-level commitments (a product-planning function informed by, but not fixed within, this engineering roadmap).

---

## 3. Architecture: The Unified Trajectory

```
STAGE 1 — Observation (MVP, Documents 01-23's full scope)
  Passive Gmail ingestion → extraction → FinancialEvent ledger
        │
        ▼
STAGE 2 — Understanding (MVP, overlapping into Phase 2)
  Categorization, Analytics, Budget tracking
        │
        ▼
STAGE 3 — Notification & Engagement (MVP, overlapping into Phase 2)
  WhatsApp summaries, alerts, savings goals
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│  ROADMAP STARTS HERE — everything below is beyond MVP scope    │
└─────────────────────────────────────────────────────────────┘
        │
        ▼
PHASE 2 — Multi-Account + Budget Depth + Deeper AI
  (§5 below)
        │
        ▼
PHASE 3 — Advisory Intelligence (STAGE 4, the Vision's north star)
  (§6 below)
        │
        ▼
HORIZON — Multi-Source Consolidation & Beyond
  (§7 below)
```

Per `02_product_requirement_document.md` Section 6.3's explicit note: this phasing is a **release-planning sequence**, not a rigid engineering-sequencing constraint — the architecture (`06_architecture.md` onward) was deliberately built from Stage 1 to already support Stage 4's data requirements, meaning the roadmap below is genuinely additive capability-building, not a series of retrofits.

---

## 4. Immediately Post-Launch: Operational Maturation (First 1–3 Months)

Before any new *feature* work, the series itself identified several items that must happen shortly after launch, using real production data — these are not new capabilities, but calibration and validation activities explicitly deferred to "after real data exists":

| Item | Source | Why Deferred to Post-Launch |
|---|---|---|
| Revisit confidence gating threshold (default 0.70) using real extraction accuracy data | `02_product_requirement_document.md` §12, `11_ai_blueprint.md` §6.3 | Cannot be meaningfully tuned without real-world calibration data |
| Recalibrate `22_risk_analysis.md`'s likelihood estimates using observed incident frequency | `22_risk_analysis.md` §16 | Design-time estimates are necessarily provisional |
| Validate `23_scalability.md`'s partitioning/quota thresholds against real volume | `23_scalability.md` §13 | Thresholds were explicitly flagged as starting estimates |
| Establish burn-rate alerting for numeric-target SLOs | `17_observability.md` §15 | Requires sufficient traffic history to tune meaningfully |
| Review alert-to-actioned ratio (alert fatigue check) | `17_observability.md` §17, `22_risk_analysis.md` R-O1 | Can only be measured once alerts have actually fired in production |
| Expand known-sender template library using Parser Monitoring data | `02_product_requirement_document.md` §6.1, `13_email_parser.md` §15 | Ongoing, data-informed content work, begins the moment real traffic exists |
| Grow the AI evaluation set from production-diagnosed failures | `18_testing_strategy.md` §8.1 | Self-correcting by design, requires real failures to learn from |

**This section exists because it would be a mistake to treat "launch" as "done"** — the series repeatedly, deliberately deferred a specific, bounded set of calibration decisions to this exact moment, and this roadmap makes that deferred work visible and owned rather than letting it quietly fall through the cracks between "launch" and "next feature."

---

## 5. Phase 2 — Multi-Account + Budget Depth + Deeper AI

Directly per `02_product_requirement_document.md` Section 6.2, enriched with the specific prior-document improvements that naturally belong in this phase:

### 5.1 Multi-Account Support
- Full `FR-USER-01` multi-Gmail-account linking (MVP shipped single-account; Phase 2 completes the roll-up-to-one-ledger experience across accounts).
- **Dependency:** None outstanding — the domain model (`04_domain_model.md` Section 6.1's `GmailAccount` aggregate) and database design already support this; it is primarily an API/product surfacing exercise plus the multi-account merge scenario sequence diagrams noted as a gap in `07_sequence_diagrams.md` Section 16.

### 5.2 Budget & Analytics Depth
- Merchant Ranking analytics (`FR-ANALYTICS-03`), Savings Goal tracking (`FR-BUDGET-03`) — both already modeled in `04_domain_model.md` and `08_database_design.md`, deferred only at the product-surfacing layer.
- Manual correction/override workflows (`FR-LEDGER-02`) reaching full product polish.

### 5.3 Deeper AI Capability
- Fine-tuned, purpose-specific categorization model, if call volume justifies the investment (`11_ai_blueprint.md` §17).
- Multi-language confidence-rubric calibration, once non-English sender support is prioritized (`11_ai_blueprint.md` §17).
- Semi-automated template-drafting assistance using AI Extractor output as a human template-author's starting point (`13_email_parser.md` §14) — a natural synergy explicitly deferred past MVP.

### 5.4 Observability & Process Maturity
- Multi-window, multi-burn-rate SLO alerting, once sufficient traffic history exists (`17_observability.md` §15).
- Mutation testing for the highest-stakes domain invariant tests (`18_testing_strategy.md` §17).
- Canary/percentage-based production rollout, once traffic volume makes canary analysis statistically meaningful (`19_deployment.md` §16).

---

## 6. Phase 3 — Advisory Intelligence (Stage 4, the Vision's North Star)

Directly per `01_product_vision.md` Section 7's Stage 4 and `02_product_requirement_document.md` Section 6.3:

### 6.1 Conversational Query Interface
- A WhatsApp-native conversational query capability ("How much did I spend on transport this week?"), explicitly foreshadowed throughout the series:
  - `05_event_storming.md` §20: a new "Conversational Query" phase with `SubmitFinancialQuery`/`FinancialQueryAnswered` commands/events, not yet modeled.
  - `09_api_specification.md` §13: a `/financial-events/query` endpoint, likely proxying to a dedicated AI-assistant service rather than the standard CRUD surface.
  - `15_notification_system.md` §15: a richer, still-structured-first reply-parsing model extending beyond Confirm/Edit, **but explicitly required to preserve the "structured-first, never open NLU for ledger-affecting replies" discipline** established in that document — this constraint carries forward into Phase 3, not just MVP.
- **Dependency:** This is the single largest net-new capability in the roadmap and warrants its own focused design pass (its own Vision-through-Architecture mini-series, following this series' own methodology) rather than being retrofitted as an afterthought onto the existing API/Engagement design.

### 6.2 AI-Driven Recommendations & Anomaly Detection
- `FR-NOTIF-04` AI Recommendation messages (e.g., "You've spent 40% more on subscriptions this quarter — here are 3 you haven't used").
- Anomaly detection: duplicate-charge patterns, fraud-like signals, failed-payment cascades (`02_product_requirement_document.md` §6.3, `05_event_storming.md` §20).
- **Dependency:** Requires a new approved WhatsApp message template (`15_notification_system.md` §4.3's explicit lead-time note applies) and a mature-enough categorization/confidence baseline (Section 4's post-launch calibration work) to avoid recommending against noisy, uncalibrated data.

### 6.3 Predictive Cash Flow Forecasting
- Per `01_product_vision.md` Section 14: "Based on your upcoming renewals and average spend, you'll be short by X this month."
- **Dependency:** Requires `SubscriptionRenewal`'s `next_expected_date` field (already modeled, `04_domain_model.md` §5.1.2) to be populated reliably at scale, and a forecasting model — likely a new, distinct AI Engine capability beyond the current extraction/categorization scope, warranting its own blueprint extension to `11_ai_blueprint.md`.

### 6.4 Richer Financial Planning Domain
- Per `04_domain_model.md` §19: promoting `Budget` and `SavingsGoal` into a richer "Financial Planning" bounded context if Phase 3's predictive features demand more sophisticated planning-domain concepts than currently modeled — an explicit note that this phase may warrant a **new bounded context**, not merely new features within the existing `budget` module.

---

## 7. Horizon — Multi-Source Consolidation & Beyond

Per `01_product_vision.md` Section 14 and the Connector abstraction's explicit design intent (`01_product_vision.md` §4.1, `06_architecture.md` §3, `12_gmail_connector.md` §1):

### 7.1 Additional Connectors
In the order the Connector abstraction (`12_gmail_connector.md` Section 3.1) most naturally extends to, per that document's own Future Improvements note (§15):
1. **Outlook / IMAP** — the most direct analog to Gmail; `12_gmail_connector.md` §15 notes the classification heuristic (currency patterns, keyword sets) is largely source-independent and should be extracted into a connector-agnostic shared library at this point.
2. **SMS-based bank alerts** — a structurally simpler source (typically short, highly templated messages) that would benefit from, but not require, the existing Template/Regex Parser DSL.
3. **Bank APIs / Payment Gateways** (where available in-market) — structured JSON sources that, per `01_product_vision.md` §10.1's original rationale, are a "specialization, not a special case" relative to the harder Gmail connector already built — these should be the **easiest** connectors to add, validating the abstraction's design payoff.
4. **PDF Statement import, CSV import, WhatsApp/Telegram as sources** — lower-priority, more bespoke parsing needs.

### 7.2 Cross-Source Deduplication
- `03_software_requirement_specification.md` §4.3 and `06_architecture.md`'s domain model already flag secondary fuzzy-match deduplication (amount + merchant + timestamp window) as a "Phase 2+" requirement once multi-connector ingestion is active — this becomes load-bearing the moment a second connector type ships, not optional polish.

### 7.3 Household / Shared Ledgers
- Per `01_product_vision.md` §14: multi-user reconciliation for shared/household finances — a genuinely new domain concept (shared ownership of a `FinancialEvent`, or a household-level aggregate root above the current per-user model) requiring its own domain-modeling pass, not a straightforward extension of the current single-owner `FinancialEvent.user_id` design.

### 7.4 Investment & Net-Worth Tracking
- Per `04_domain_model.md` §19: richer `Investment`/`Dividend` modeling (holdings, cost basis) as a natural extension once net-worth tracking is prioritized.

### 7.5 Open Banking
- Per `01_product_vision.md` §15: Bank API/Open Banking connectors in geographies where available, reducing reliance on email parsing entirely for those sources — the long-term trajectory of Section 7.1's connector list, where the hardest-to-parse source (email) is gradually supplemented by the most structurally reliable one (direct bank data).

---

## 8. Cross-Cutting Technical Debt & Hardening Items

Items from the series' "Future Improvements" sections that aren't tied to a specific product phase, but should be picked up opportunistically as capacity allows:

| Item | Source |
|---|---|
| Replace polling Outbox Relay with Postgres CDC-based relay, if latency/load become bottlenecks | `06_architecture.md` §18 |
| Dedicated append-only event-log/replay store, if BI/data-warehouse export needs grow | `06_architecture.md` §18, `08_database_design.md` §19 |
| HSM-backed KMS evaluation, if a higher assurance level is required | `16_security.md` §17 |
| Field-level encryption for Tier 2 identifiers, if a regional regulation mandates it | `16_security.md` §17 |
| Automated GDPR erasure-cascade verification as a continuous (not just pre-launch) production job | `16_security.md` §17, `18_testing_strategy.md` §19 |
| WhatsApp "Flows" feature evaluation for richer structured corrections | `15_notification_system.md` §15 |
| Chaos-engineering test suite targeting the failure-domain table | `18_testing_strategy.md` §17 |
| Generate PR checklist as an actual GitLab template file | `20_coding_convention.md` §15 |
| Risk-register citation-freshness check (automated) | `22_risk_analysis.md` §16 |

---

## 9. Detailed Explanation: Why This Document Deliberately Under-Specifies Future Phases

Every prior document in this series was held to a "never simplify the design" standard — full invariants, full sequence diagrams, full schema detail. This document, by contrast, deliberately stops short of that same depth for Phase 2/3/Horizon items. This is not a lapse in rigor; it is a considered decision:

1. **Premature over-specification of a not-yet-prioritized feature is its own form of waste and risk.** A fully-detailed domain model for, say, Household Ledgers (Section 7.3) written today, before any real user demand signal or design iteration has occurred, is likely to be wrong in ways that are expensive to unwind later — exactly the inverse of this series' own stated principle of designing *now* only what's being built *now*, while keeping the architecture *extensible* for later (the Connector abstraction, the `FinancialEvent` taxonomy's `Others` catch-all, the module-boundary design) rather than *pre-building* later features early.
2. **This document's actual job is different from Documents 01–23's job.** Those documents answer "how do we build this specific thing correctly." This document answers "what should we build next, and in what order, given what we now know" — a roadmap's value is in prioritization and dependency-sequencing, not implementation detail, which is exactly why, per Section 2.1, a future phase's *own* detailed specification should follow this series' own methodology **when that phase is actually scheduled** — this document is the trigger and the map, not the specification itself.
3. **This mirrors the Vision document's own explicit design philosophy, applied reflexively to this very series:** `01_product_vision.md` Section 7 stated the product's maturity curve (Observation → Understanding → Engagement → Advisory) as a deliberate staging, not a feature list to build all at once. This roadmap document is that same staging discipline, applied now to the engineering *planning process itself* — knowing what to detail now and what to defer is the same judgment the whole project has exercised about the product's own capabilities.

---

## 10. Component Responsibilities

| Component | Responsibility |
|---|---|
| This roadmap | Prioritized sequencing and dependency-mapping of post-MVP work; explicitly not detailed specification |
| Section 4 (Post-Launch Maturation) | Owns the calibration/validation work explicitly deferred by name throughout the series |
| Section 5–7 (Phase 2/3/Horizon) | Own directional sequencing and dependency identification for major future capability |
| Section 8 (Technical Debt) | Owns opportunistic, non-phase-gated hardening work |
| A future phase's own design-document pass | Owns the actual implementation-ready specification, when that phase is scheduled |

---

## 11. Trade-offs

| Decision | Trade-off |
|---|---|
| Deliberately lighter specification depth for future-phase items | Less immediately actionable for an engineer picking up a Phase 2/3 item today, in exchange for not over-investing design effort in features that may yet change based on real user feedback and production learnings. |
| Treating Section 4's post-launch calibration work as explicitly roadmapped (not just implicitly assumed) | Adds visible roadmap items that aren't new "features" in the usual sense, in exchange for ensuring this deferred-by-design work is actually tracked and owned rather than falling through the cracks of a feature-only backlog. |
| Recommending a full mini-series design pass for the Conversational Query capability (Section 6.1) specifically | A larger upfront design investment for that one feature relative to other Phase 3 items, in exchange for recognizing it as genuinely the largest, most architecturally consequential net-new capability in the entire roadmap, warranting proportionate rigor when its time comes. |

## 12. Advantages
- This document closes the entire twenty-four-document series with a clear answer to "what's next," preventing the common failure mode where an exhaustive design series ships and then the team has no clear, prioritized sense of what to build after launch beyond ad hoc feature requests.
- Consolidating every scattered "Future Improvement" note into one place (Section 8 especially) ensures none of that accumulated, document-specific foresight is lost or forgotten once the individual documents stop being actively read day-to-day.
- The explicit dependency-mapping (e.g., Section 6.2's recommendation engine depending on Section 4's calibration work) gives planning a genuine, reasoned sequence rather than an arbitrary feature-priority list.

## 13. Disadvantages
- A roadmap, by nature, is the part of this series most likely to go stale fastest — real user feedback and market conditions post-launch will likely reprioritize some of Sections 5–7's ordering, requiring this document to be revisited and revised more frequently than, say, `04_domain_model.md`'s foundational decisions.
- The deliberately lighter specification depth (Section 9) means some stakeholders may perceive this document as less "complete" than its predecessors — a perception worth proactively managing by clearly communicating Section 9's rationale, rather than letting it read as an unexplained drop in rigor.

## 14. Future Improvements
- *(On this document itself, reflexively)*: Revisit this roadmap's Section 5–7 prioritization at least once per quarter, incorporating real post-launch user feedback, support-ticket themes, and the Section 4 calibration findings — this document should be treated as a living artifact, re-ranked as real evidence accumulates, not a fixed prophecy written once at the end of the design phase.
- When Phase 2 is formally scheduled, spin up a focused addendum (or a Documents 25+ continuation, following this same series' structure) for its specific scope, rather than retrofitting detail into this already-closing document.
- Consider formally tracking Section 8's technical-debt items in the same engineering backlog/ticketing system used for feature work, ensuring they compete fairly for prioritization attention rather than being a document-only, easily-ignored list.

## 15. Best Practices
- No new roadmap item should be added anywhere in the backlog without checking whether it's already anticipated somewhere in this document (or its Section 8 technical-debt catalog) — if it is, link to the existing entry and its dependency context rather than creating an orphaned, context-free ticket.
- Any future-phase work that begins should explicitly reference which Section 5/6/7 item it corresponds to and verify its originally-identified dependencies (e.g., Section 6.1's Conversational Query work should explicitly verify Section 4's calibration maturity before beginning) are genuinely satisfied before starting.
- This document's existence and location should be referenced in onboarding material for any new engineer or product stakeholder joining the project, as the canonical "here's where we're headed" answer, keeping it a living, referenced artifact rather than a document nobody returns to after initial release.

## 16. Production Considerations
- Section 4's post-launch maturation items are not optional "nice to haves" — several (confidence threshold recalibration, risk-register likelihood updates) directly affect the correctness and risk posture established throughout the series, and should be explicitly scheduled and owned from the moment of launch, not left to "whenever someone gets to it."
- Phase 3's Conversational Query capability (Section 6.1) carries forward the series' hardest-won discipline — "structured-first, never open NLU for ledger-affecting replies" — as a non-negotiable constraint on its eventual design, not a suggestion; this must be explicitly re-stated and defended when that feature's own dedicated design series begins, since it will be tempting, given LLM capability advances between now and then, to relax it.
- The Connector abstraction's real test (Section 7.1) comes with the *second* connector (Outlook/IMAP). Its build should be treated as a validation exercise for the entire series' core architectural promise — if adding a second connector requires significant rework of `financialcore`, `analytics`, `budget`, or `engagement`, that is a signal the module-boundary discipline established across Documents 04–10 was not actually upheld during MVP implementation, and warrants a dedicated architecture review before proceeding further down the connector roadmap.

---

## 17. Closing Note

This document concludes the twenty-four-document Financial Event Platform specification series. Documents 01–23 define a complete, production-grade blueprint for a system that treats a user's financial life as a continuous, multi-source stream of typed `FinancialEvent` occurrences — observed passively, interpreted through a deterministic-first, AI-augmented pipeline, recorded in an auditable ledger, and communicated proactively through WhatsApp — built on a modular-monolith Go backend with the architectural seams already in place for the system to grow, module by module and connector by connector, toward the Vision's Stage 4 north star: a trusted, conversational financial assistant.

This document — the roadmap — is the bridge from that blueprint to what comes after it is built. The series is complete; the platform's evolution begins where this document ends.
