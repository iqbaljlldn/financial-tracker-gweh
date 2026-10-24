# Product Vision Document

## Personal Finance Assistant Platform — "Financial Event Platform"

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Owning Team:** Platform Architecture Group
**Related Documents:** 02_product_requirement_document.md, 04_domain_model.md, 06_architecture.md

---

## 1. Purpose

This document defines the **why** behind the platform before any team defines the **what** (PRD/SRS) or the **how** (Architecture/Backend Blueprint). It exists to align every subsequent engineering decision — schema design, event modeling, connector architecture, AI extraction strategy — around a single, unambiguous product identity.

The purpose of this document is to:

1. Establish the platform's identity as a **Financial Event Platform**, not an expense tracker.
2. Define the problem space, target users, and value proposition.
3. Establish the guiding principles that all 24 downstream documents must respect.
4. Prevent architectural drift — a common failure mode where a system initially designed as a "simple expense logger" becomes structurally incapable of supporting AI-driven automation, multi-source ingestion, or event-driven analytics later on.
5. Give every engineering discipline (backend, AI, DevOps, security, database) a shared mental model of the domain **before** they touch schema or code.

This is a strategic document. It intentionally avoids implementation detail. Anywhere this document says "the system should," subsequent documents (SRS, Architecture, Database Design) are responsible for saying "the system does this via X mechanism."

---

## 2. Scope

### 2.1 In Scope for This Document

- Product framing and naming philosophy (Financial Event Platform vs. Expense Tracker).
- Problem statement and target personas.
- Core value propositions and differentiators.
- Guiding architectural principles derived from the vision (not the architecture itself).
- Domain philosophy: why `FinancialEvent` is the aggregate root of the entire system, not `Transaction`.
- Long-term product trajectory (automation → intelligence → advisory).
- Success criteria at the vision level (not KPIs — those belong in the PRD).

### 2.2 Out of Scope for This Document

- Detailed functional requirements → `02_product_requirement_document.md`
- Non-functional/quality attribute specifications → `03_software_requirement_specification.md`
- Entity relationship modeling → `04_domain_model.md`, `08_database_design.md`
- Technology stack justification → `06_architecture.md`, `10_backend_blueprint.md`
- API contracts → `09_api_specification.md`
- Any code, schema DDL, or pseudocode.

---

## 3. Problem Statement

### 3.1 The Core Problem

Individuals and households generate a continuous stream of **financial events** — income deposits, purchases, transfers, subscription renewals, refunds, failed payments, credit card bills, investment postings, dividends, tax deductions — across **many disconnected surfaces**: bank apps, e-wallets, credit card statements, brokerage platforms, and merchant emails.

Traditional personal finance tools require the user to be the integration layer: manually opening an app, typing an amount, selecting a category, attaching a note. This creates three structural failures:

1. **Abandonment** — manual logging has near-universal drop-off after 2–3 weeks (well documented across the personal finance app category).
2. **Incompleteness** — users log what they _remember_, not what actually happened. Subscription renewals, bank fees, and passive income are systematically under-recorded.
3. **Delayed insight** — by the time a user manually reconciles their spending, the financial decision window (e.g., "should I make this purchase") has already closed.

### 3.2 The Insight

Nearly every financial event a person generates **already produces a digital notification** — most commonly, an email from a bank, e-wallet, or payment processor. This is a passive, structured (or semi-structured), timestamped, and nearly complete record of the event itself.

**Gmail is not a feature. Gmail is the first of many transaction sources.** The architectural insight this document insists on: the platform's job is to **observe financial events wherever they are emitted**, extract structured meaning from unstructured or semi-structured signals, and turn that into a queryable, analyzable, actionable ledger — with zero manual entry as the default experience.

### 3.3 Why "Expense Tracker" Framing Is Rejected

An expense tracker assumes:

- The user is the primary data source.
- Every record is a discrete, isolated expense.
- The domain model is flat (`amount`, `category`, `date`).
- The product's ceiling is a pie chart.

This platform explicitly rejects that ceiling. The domain is reframed around **`FinancialEvent`** — a typed, extensible, source-agnostic representation of _any_ financially significant occurrence in a user's life, including but not limited to:

```

FinancialEvent
├── Income
├── Expense
├── Transfer
├── Refund
├── Cashback
├── Subscription Renewal
├── Failed Payment
├── Salary
├── Investment
├── Dividend
├── Loan
├── Credit Card Bill
├── Tax
├── Donation
└── Others (extensible)

```

This is not a naming preference — it is a **structural commitment**. Every downstream document (domain model, database design, API spec, AI blueprint) must treat `FinancialEvent` as the aggregate root, with `Transaction`-like records being one _projection_ or _view_ of that root, not the root itself. This decision is discussed in depth in `04_domain_model.md`.

---

## 4. Product Identity

> **The platform is a Financial Event Platform: a system that observes, understands, and acts on the financial events of a user's life, sourced from any channel, with AI as the interpretive layer between raw signal and structured meaning.**

It is **not**:

- A budgeting app (budgeting is one _capability_ built atop the event ledger — see Budget Engine in `02_product_requirement_document.md`).
- An expense tracker (expense is one of many `FinancialEvent` types).
- A Gmail-reading tool (Gmail is Connector #1 of an extensible connector framework).
- A chatbot (WhatsApp/AI assistant is one _interface_ into the platform, not the platform itself).

### 4.1 Product Identity Pillars

| Pillar                                           | Description                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Source-Agnostic Ingestion**                    | The system is built from day one with a Connector abstraction. Gmail is the reference implementation; Outlook, IMAP, SMS, WhatsApp, Telegram, CSV, Bank APIs, Payment Gateways, and PDF Statements are first-class future citizens, not afterthoughts.                                                                                                                                                                 |
| **Event-Centric Domain**                         | Every observed occurrence becomes a `FinancialEvent`. The ledger, analytics, budget engine, and notifications are all downstream consumers of this single event stream — never separate silos.                                                                                                                                                                                                                         |
| **AI as an Interpretation Layer, Not a Gimmick** | AI is used specifically where deterministic parsing fails (unknown senders, novel email formats, ambiguous merchant names) and where synthesis is required (categorization, recommendations, natural-language summaries). Known, high-volume senders use deterministic template/regex parsing for cost, latency, and reliability — AI is the fallback and the enrichment layer, not the primary parser for everything. |
| **Progressive Autonomy**                         | The system starts by faithfully recording what happened (ledger), then explains what happened (analytics), then warns about what might happen (budget alerts), and ultimately advises on what the user _should_ do (AI financial assistant). This is a deliberate maturity curve, not a feature list.                                                                                                                  |
| **Trust Through Auditability**                   | Because the system acts on the user's financial life autonomously, every extraction, categorization, and notification must be traceable to its source (raw email, parser version, prompt version, confidence score). Autonomy without auditability is not acceptable for a finance product.                                                                                                                            |

---

## 5. Target Users & Personas

### 5.1 Primary Persona — "The Passive Recorder"

A working professional with multiple bank accounts, e-wallets, and credit cards, who wants an accurate financial picture without behavioral change. They will not manually log a single transaction. Success for this user = "I connected my Gmail once, and now I just get a WhatsApp summary."

### 5.2 Secondary Persona — "The Budget-Conscious Household Manager"

Manages shared or family finances, wants category-level visibility, overspending alerts, and monthly trend reports to make decisions (e.g., cutting a subscription, adjusting savings goals).

### 5.3 Tertiary Persona — "The Financially Curious Power User"

Wants merchant-level ranking, cash flow analysis, investment/dividend tracking, and eventually, conversational AI queries like _"How much did I spend on food delivery this month compared to last month?"_

### 5.4 Future Persona — "The Multi-Source Consolidator" (Roadmap)

A user with financial signals across Gmail, Outlook, SMS-based bank alerts, and manual CSV bank statements, who wants **one unified ledger** regardless of source. This persona is why the Connector abstraction cannot be Gmail-specific in its design, even in v1.

---

## 6. Value Proposition

| For                      | Who                                                | The Product Is             | That                                                                                                   | Unlike                                                                                               |
| ------------------------ | -------------------------------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| Individuals & households | who generate financial events across many channels | a Financial Event Platform | passively ingests, understands, and summarizes their entire financial life with near-zero manual input | expense trackers that require manual entry and only understand "categories," not financial semantics |

### 6.1 Differentiators

1. **Zero-friction onboarding** — Connect Gmail via OAuth once; no manual transaction entry ever required for the primary flow.
2. **Deterministic-first, AI-fallback parsing** — cost-efficient and reliable for known senders (banks, e-wallets), intelligent for the long tail of unknown senders.
3. **Multi-channel notification-native** — WhatsApp as the primary delivery surface (not just email/push), meeting users where they already are.
4. **Extensible by design** — every architectural document in this series treats "add a new connector" or "add a new FinancialEvent type" as a first-class, low-friction operation, not a rewrite.
5. **Audit-grade trust** — confidence scoring, prompt versioning, and validation gates before anything is written to the ledger, because financial data errors have real consequences.

---

## 7. High-Level Product Trajectory

The vision explicitly frames the product as evolving through four maturity stages. This staging directly informs the prioritization in `24_future_roadmap.md` and the modularity requirements in `06_architecture.md`.

### Stage 1 — **Observation** (MVP)

Passive ingestion from Gmail, parsing/extraction, normalization into `FinancialEvent`, storage in the ledger. The user's financial life becomes _visible_ without effort.

### Stage 2 — **Understanding**

Categorization (AI + rules), analytics (cash flow, category breakdown, merchant ranking, trends), and budget tracking against the ledger. The user's financial life becomes _legible_.

### Stage 3 — **Notification & Engagement**

WhatsApp-delivered daily/weekly/monthly summaries, overspending alerts, savings goal tracking. The user's financial life becomes _proactively communicated_, without the user opening the app.

### Stage 4 — **Advisory Intelligence** (North Star)

An AI financial assistant capable of conversational queries, personalized recommendations ("You've spent 40% more on subscriptions this quarter — here are 3 you haven't used"), anomaly detection (fraud-like patterns, duplicate charges, failed payment cascades), and eventually predictive cash flow forecasting. The user's financial life becomes _advised_.

> **Architectural implication:** Because Stage 4 is the north star, the domain model, event schema, and AI blueprint must be designed in Stage 1 to support Stage 4's data requirements retroactively. Retrofitting an advisory AI onto a flat "expense log" schema is the single most common reason personal finance products fail to reach this stage. This is why `FinancialEvent` — rich, typed, source-traceable, confidence-scored — is non-negotiable from day one.

---

## 8. Guiding Architectural Principles (Derived from Vision)

These principles are **binding constraints** on every downstream document:

1. **Connector Abstraction First** — No document may design Gmail ingestion in a way that couples the domain model, parser interface, or ledger to Gmail-specific concepts (message IDs, MIME structure) at the domain layer. Gmail-specific concerns are confined to the Gmail Connector module (`12_gmail_connector.md`).
2. **FinancialEvent Is the Aggregate Root** — `Transaction` (income/expense/transfer/refund) is a subtype/category within `FinancialEvent`, not a parallel or competing concept. All analytics, budget, and notification modules consume the `FinancialEvent` stream, never a source-specific structure.
3. **Deterministic Before Probabilistic** — Known senders use template/regex parsers (fast, cheap, deterministic, testable). AI extraction is invoked only for unknown senders or low-confidence deterministic parses. This is a cost, latency, and reliability decision as much as a product one.
4. **Nothing Reaches the Ledger Unvalidated** — Every extracted event carries a confidence score and passes through a validation gate before persistence. Low-confidence events are queued for review or reconciliation, never silently written as fact.
5. **Idempotency and Exactly-Once Semantics** — Because the system reads a continuously changing external mailbox, duplicate delivery (Gmail push notifications, retries, re-sync) must never produce duplicate financial events. This is a hard non-functional requirement, not an optimization.
6. **Every Automated Decision Is Explainable** — Categorization, extraction, and recommendations must be traceable to a parser version, prompt version, and confidence score — supporting both user trust and engineering debuggability.
7. **Notification Is a Projection, Not a Trigger** — WhatsApp summaries are read models derived from the ledger and analytics layers; they must never be a side-channel source of truth.
8. **Modular Monolith, Microservice-Ready** — The system is built as a modular monolith with clean module boundaries (Connector, Parser, AI Engine, Ledger, Analytics, Budget, Notification) so that any module can be extracted into an independent service without a domain rewrite, should scale demand it.

---

## 9. Architecture (Vision-Level View)

At the vision level, the architecture is expressed as a **conceptual pipeline**, not a technical one (technical architecture is fully specified in `06_architecture.md`):

```

                        ┌────────────────────────┐
                        │   Transaction Sources    │
                        │  (Gmail, future: Outlook,│
                        │   IMAP, SMS, WhatsApp,   │
                        │   Telegram, CSV, Bank    │
                        │   API, Payment Gateway,  │
                        │   PDF Statement)         │
                        └───────────┬────────────┘
                                    │  Connector Abstraction
                                    ▼
                        ┌────────────────────────┐
                        │   Signal Ingestion       │
                        │  (Watch / Sync / Fetch)  │
                        └───────────┬────────────┘
                                    ▼
                        ┌────────────────────────┐
                        │   Extraction Layer       │
                        │  (Template/Regex Parser  │
                        │   OR AI Extractor)       │
                        └───────────┬────────────┘
                                    ▼
                        ┌────────────────────────┐
                        │  Normalization +         │
                        │  Confidence Scoring +    │
                        │  Validation              │
                        └───────────┬────────────┘
                                    ▼
                        ┌────────────────────────┐
                        │   FinancialEvent Ledger  │
                        │   (Aggregate Root)       │
                        └───────────┬────────────┘
                                    ▼
              ┌─────────────────────┼─────────────────────┐
              ▼                     ▼                     ▼
     ┌────────────────┐   ┌────────────────┐   ┌────────────────────┐
     │  AI Categorization│   │   Analytics    │   │   Budget Engine     │
     └────────────────┘   └────────────────┘   └────────────────────┘
              │                     │                     │
              └─────────────────────┼─────────────────────┘
                                    ▼
                        ┌────────────────────────┐
                        │  Notification Layer      │
                        │  (WhatsApp: Daily/Weekly/│
                        │   Monthly, Alerts, AI    │
                        │   Recommendations)        │
                        └────────────────────────┘

```

This diagram is intentionally source-agnostic and technology-agnostic. It is the shape every future connector must fit into without modification to the Ledger, Analytics, Budget, or Notification layers.

---

## 10. Detailed Explanation

### 10.1 Why Gmail First, But Not Gmail-Only

Gmail is chosen as the reference connector because:

- It has the highest coverage of financial notification emails across banks, e-wallets, and merchants globally.
- Google's OAuth + Gmail API (Watch/History API) provides a mature, well-documented push-based ingestion mechanism, enabling near-real-time processing without polling.
- It validates the hardest version of the problem first: unstructured/semi-structured HTML email parsing at scale, across an unbounded set of senders — a harder problem than, say, a structured Bank API integration would be.

Solving the hardest connector first (Gmail) and designing its interface generically means every future connector (which will typically be _simpler_ — e.g., a Bank API returns structured JSON) is a **specialization**, not a special case.

### 10.2 Why AI Is Positioned as a Fallback, Not the Default Extractor

For known senders (e.g., a specific bank's transaction alert template), deterministic parsing is:

- **Cheaper** — no LLM inference cost per email.
- **Faster** — regex/template matching completes in milliseconds.
- **More reliable** — deterministic outputs are 100% reproducible and easily unit-tested.
- **More auditable** — a template version can be pinned and diffed like code.

AI extraction is reserved for:

- Previously unseen senders/templates.
- Low-confidence or failed deterministic parses.
- Semantic enrichment tasks that are inherently probabilistic (categorization, merchant normalization, natural-language recommendation generation).

This hybrid strategy is elaborated fully in `11_ai_blueprint.md` and `13_email_parser.md`.

### 10.3 Why WhatsApp as the Primary Notification Channel

WhatsApp is chosen over push notifications or email digests because:

- It has near-universal daily engagement in the platform's target markets.
- It supports rich, conversational delivery (daily/weekly/monthly summaries can be read as a message, not require opening an app).
- It sets up the long-term vision of a **conversational financial assistant** — the same channel used for summaries can later support "ask a question" interactions.

### 10.4 Why "FinancialEvent" and Not "Transaction" as the Root Concept

A `Transaction` implies a monetary movement with a counterparty and an amount. But several financially significant occurrences do not fit that shape cleanly:

- A **Failed Payment** has no successful monetary movement, but is highly informative (signals a card issue, insufficient funds, or fraud attempt).
- A **Subscription Renewal** may need to be tracked even before it succeeds/fails, to support "upcoming renewal" alerts.
- A **Credit Card Bill** is a summary/statement event, not a discrete transaction, but must be tracked to reconcile against underlying expense events.
- **Tax** and **Donation** events may need special regulatory/reporting treatment distinct from a generic expense.

By modeling `FinancialEvent` as the root with `Transaction`-type events as one subtype family, the system retains the flexibility to represent all these cases without schema contortions later. This decision is carried through in detail in `04_domain_model.md` and `08_database_design.md`.

---

## 11. Component Responsibilities (Vision-Level)

| Component                  | Responsibility                                                                        | Owning Document                               |
| -------------------------- | ------------------------------------------------------------------------------------- | --------------------------------------------- |
| Connector Framework        | Abstract interface for any transaction source; Gmail is the first implementation      | `12_gmail_connector.md`                       |
| Extraction Layer           | Convert raw signal (email, SMS, PDF) into candidate structured data                   | `13_email_parser.md`, `11_ai_blueprint.md`    |
| Normalization & Validation | Ensure extracted data conforms to the `FinancialEvent` schema, assign confidence      | `04_domain_model.md`, `08_database_design.md` |
| FinancialEvent Ledger      | System of record for all financial events, source-agnostic                            | `08_database_design.md`                       |
| AI Categorization          | Assign/refine category, merchant normalization, confidence-aware                      | `11_ai_blueprint.md`                          |
| Analytics Engine           | Derive cash flow, trends, merchant ranking from the ledger                            | `02_product_requirement_document.md`          |
| Budget Engine              | Track monthly budgets, savings goals, overspending detection                          | `02_product_requirement_document.md`          |
| Notification Layer         | Deliver summaries/alerts via WhatsApp                                                 | `15_notification_system.md`                   |
| Administration             | Observability into connector health, parser accuracy, prompt performance, audit trail | `16_security.md`, `17_observability.md`       |

---

## 12. Trade-offs

| Decision                                                                     | Trade-off Accepted                                                                                                                                                                             |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Event-centric domain (`FinancialEvent` root) over simple `Transaction` table | Higher upfront modeling complexity in exchange for long-term extensibility and avoidance of costly schema migrations later.                                                                    |
| Deterministic-first parsing with AI fallback                                 | Requires maintaining two extraction paths (templates + AI) rather than one, in exchange for cost control and reliability at scale.                                                             |
| Gmail-first connector strategy                                               | Slower "multi-source" story at launch, in exchange for validating the hardest ingestion problem first and de-risking the connector abstraction.                                                |
| WhatsApp as primary notification channel                                     | Introduces a third-party messaging dependency and compliance surface (WhatsApp Business API policies), in exchange for materially higher user engagement than email/push.                      |
| Modular monolith (not microservices at launch)                               | Slightly higher coupling risk within a single deployable, in exchange for dramatically lower operational overhead during early product stages, while preserving a clean extraction path later. |
| Confidence scoring + validation gate on every event                          | Adds latency and complexity to the ingestion pipeline, in exchange for protecting the integrity of a financial ledger — a domain where correctness is non-negotiable.                          |

---

## 13. Advantages

- **Zero manual entry** is a genuine, defensible differentiator against every category incumbent (Mint-likes, Toshl, Wallet apps) that still fundamentally requires manual logging.
- **Event-centric domain** future-proofs the platform against feature requests that would otherwise force a schema rewrite (investment tracking, tax events, credit bill reconciliation).
- **Connector abstraction** turns "add a new data source" into a roadmap item, not a re-architecture.
- **Hybrid deterministic/AI extraction** keeps unit economics sane at scale (most volume from a small number of known senders) while still handling the long tail gracefully.
- **WhatsApp-native delivery** meets users in their most-used app, materially increasing the odds of sustained engagement without requiring the user to open the platform's own app.

## 14. Disadvantages

- **Higher initial engineering complexity** than a CRUD expense tracker — this is a deliberate, accepted cost per the vision, but it does mean a longer path to a minimal viable release.
- **Dependency on third-party platforms** (Gmail API quotas/policies, WhatsApp Business API approval and pricing) introduces external risk the team does not fully control — addressed in `22_risk_analysis.md`.
- **AI extraction introduces non-determinism** into a financial product, requiring careful confidence-gating, human-in-the-loop reconciliation flows, and prompt versioning discipline to maintain trust — addressed in `11_ai_blueprint.md`.
- **Email parsing is inherently fragile** to sender template changes, requiring ongoing parser maintenance and monitoring — addressed in `13_email_parser.md` and `16_administration/parser monitoring`.

---

## 15. Future Improvements (Vision-Level)

- Expansion into full multi-source consolidation (Outlook, SMS, IMAP, Bank APIs) with a unified cross-source deduplication strategy.
- Conversational AI assistant accessible directly via WhatsApp ("How much did I spend on transport this week?").
- Predictive cash flow forecasting and proactive financial advisory ("Based on your upcoming renewals and average spend, you'll be short by X this month").
- Shared/household ledgers with multi-user reconciliation.
- Investment and net-worth tracking as a natural extension of the `Investment`/`Dividend` FinancialEvent types.
- Open banking / Bank API connectors in geographies where available, reducing reliance on email parsing entirely for those sources.

Full roadmap detail is in `24_future_roadmap.md`.

---

## 16. Best Practices Established by This Vision

1. Treat every new transaction source as a **Connector implementation**, never a special case bolted onto the core.
2. Treat every new financially significant occurrence as a **new FinancialEvent subtype**, never a new parallel table.
3. Never let AI write directly to the ledger without passing through confidence scoring and validation.
4. Never treat notification content as a source of truth — it is always a derived read model.
5. Design every module boundary as if it could become a separate microservice tomorrow, even while shipping it as a modular monolith today.

---

## 17. Production Considerations

- This vision assumes handling of **sensitive financial data** end-to-end; every downstream document must treat encryption at rest/in transit, least-privilege OAuth scopes, and audit logging as baseline requirements, not enhancements (see `16_security.md`).
- The system must be designed for **GDPR-friendly** data handling from the outset (user data export, deletion, and processing transparency), since financial and email content are both sensitive personal data categories.
- Because the system acts on a continuously live external mailbox, **idempotency and exactly-once processing** are production-blocking requirements, not nice-to-haves — Gmail push notifications, retries, and re-syncs are expected to produce duplicate signals, and the system must be structurally immune to this.
- Vendor dependency risk (Gmail API, WhatsApp Business API, LLM provider) must be tracked and mitigated at the architecture level (interface abstraction, provider-agnostic AI client) so the platform is not structurally locked to a single vendor's API surface.

---

## 18. Summary Statement

This platform is not being built to help users log expenses faster. It is being built to **passively observe, structurally understand, and proactively communicate the full financial life of a user**, starting from Gmail as the first of many observation channels, using AI precisely where deterministic logic cannot reach, and culminating in a trusted, conversational financial assistant. Every document that follows this one exists to make that vision buildable, testable, secure, and scalable in production.

---
