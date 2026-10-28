# Frontend Vision
## Personal Finance Assistant Platform — Financial Event Platform (Frontend)

**Document Version:** 1.0
**Status:** Draft for Engineering Review
**Depends On:** `01_product_vision.md`, `02_product_requirement_document.md`, `09_api_specification.md`
**Feeds Into:** `26_frontend_product_requirement_document.md`, all subsequent frontend documents

---

## 1. Purpose

This document opens the **frontend document series** — the continuation of the Financial Event Platform's 24-document backend specification, now extending the same rigor to the client application. Where `01_product_vision.md` established *why the platform exists* and *what the backend must guarantee*, this document establishes *why the frontend exists as a distinct, carefully-designed surface*, not merely "a UI that calls the API."

This series does not re-derive the product's identity — `01_product_vision.md`'s Financial Event Platform framing, the Stage 1–4 maturity trajectory, and the zero-manual-entry promise all carry forward unchanged. What this document adds is the frontend's **specific role** in realizing that vision, and the engineering discipline that role demands.

## 2. Scope

### 2.1 In Scope
- The frontend's product identity and relationship to the backend series.
- Stack decision rationale: React + Vite, Web app + PWA.
- Guiding principles specific to the frontend (derived from, and consistent with, the backend series' principles).
- How this series reconciles with the frontend tasks already embedded in the sprint checklist series (`sprint-01` through `sprint-12`).

### 2.2 Out of Scope
- Backend architecture, domain model, or API contract detail (already fully specified in Documents 01–24; referenced, never restated).
- Concrete screen-by-screen specification → `31_screen_specifications.md`.
- Component-level or state-management detail → `29_frontend_architecture.md` onward.

---

## 3. Frontend Identity: What the Frontend Is, and Is Not

> **The frontend is the primary interactive surface through which a user directly observes, confirms, and acts on their Financial Event ledger — a companion to, never a replacement for, the platform's passive, WhatsApp-delivered engagement model.**

It is **not**:
- The platform's only interface — per `01_product_vision.md` and `15_notification_system.md`, WhatsApp is the primary *proactive* engagement channel; the frontend is the primary *pull-based* (user-initiated) surface. Both exist because they serve different moments: WhatsApp delivers insight to the user without effort; the frontend is where the user goes when they want to look deeper, correct something, or configure something.
- A thin, interchangeable "admin panel" bolted onto the API — it is a first-class product surface, deserving the same design rigor (information architecture, state management discipline, accessibility, offline behavior) as the backend received architectural rigor.
- A generic CRUD dashboard — per the backend's `FinancialEvent`-centric domain philosophy, the frontend's information architecture (`28_frontend_information_architecture.md`) is organized around financial *events* and their lifecycle (extracted → pending review → finalized → categorized), not around generic database tables.

### 3.1 Relationship to the Sprint Checklist Series

The `sprint-01` through `sprint-12` checklists already embedded a "Frontend (Parallel Track)" section in each sprint, listing specific screens tied to each backend capability as it became available (login, Gmail linking, ledger view, budget forms, analytics dashboard, notification preferences). **This frontend document series does not discard or re-derive that work** — it is the authoritative, consolidated specification *underneath* those checklist items. `31_screen_specifications.md` explicitly enumerates every screen named across the sprint checklists and gives each the same implementation-ready depth the backend series gave every API endpoint. The sprint checklists remain the sequencing/task-tracking artifact; this series is the design authority they implement against — the same relationship the backend's `02_product_requirement_document.md` has to its own eventual sprint planning.

---

## 4. Stack Decision: React + Vite

| Choice | Rationale |
|---|---|
| **React** | Matches the primary stack already in active use (per the engineering team's existing familiarity), has mature support for the data-fetching, forms, and charting libraries this domain needs (decimal-safe money handling, multi-currency chart rendering), and has the broadest PWA tooling ecosystem maturity of the mainstream options. |
| **Vite** | Fast dev-server iteration and a lean, modern build pipeline; first-class, low-friction PWA plugin support (`vite-plugin-pwa`), which is a direct, load-bearing requirement given Section 5's PWA scope decision — not an incidental convenience. |
| **TypeScript (implied, formalized in `29_frontend_architecture.md`)** | The backend's API contract (`09_api_specification.md`) is strictly typed at the Go/JSON boundary (camelCase, decimal-string money, explicit enums for `eventType`/`status`); a typed frontend is the only way to keep that contract's guarantees from silently eroding at the UI boundary — mirrors the backend's own "favor explicitness over implicit typing" posture (sqlc over a full ORM, explicit DTO mapping over reflection-based auto-mapping, per `10_backend_blueprint.md` §10). |

---

## 5. Scope Decision: Web App + PWA

The frontend ships as an **installable Progressive Web App**, not a plain web app and not a native mobile app, because:

1. **The product's core promise is passive, low-friction engagement** (`01_product_vision.md` §4) — a PWA that can be added to a user's home screen, receives basic offline-tolerant behavior, and feels app-like without requiring an app-store install is the lowest-friction way to extend that promise to the pull-based surface, without the engineering and distribution overhead of a native mobile app.
2. **It does not compete with WhatsApp as the primary proactive channel** — the PWA is explicitly the secondary, on-demand surface (Section 3); it does not need native push notifications or deep OS integration to fulfill its role, which keeps "PWA, not native app" a sufficient, not merely a compromise, choice for MVP scope.
3. **A native mobile app remains a clearly-separated future option**, consistent with `24_future_roadmap.md`'s own discipline of not over-building ahead of evidence — if a native app is ever warranted, it is additive to this series, not a replacement of it.

Full PWA behavior (manifest, service worker caching strategy, offline/stale-data UX, installability prompts) is specified in `36_pwa_specification.md`, not here — this document only establishes *that* it's in scope and *why*.

---

## 6. Guiding Principles (Derived From, and Consistent With, the Backend Series)

These principles are binding on every subsequent frontend document, directly inherited from `01_product_vision.md` §8's backend principles, translated to the frontend's concerns:

1. **The API Contract Is the Only Boundary.** The frontend never queries a database, never calls Gmail/WhatsApp/the LLM provider directly, and never assumes a data shape beyond what `09_api_specification.md` documents. This mirrors the backend's module-boundary discipline (`06_architecture.md` §5) applied at the frontend/backend seam.
2. **Money Is Never a JavaScript Float.** Per `09_api_specification.md` §14, every monetary value crosses the wire as a decimal string. The frontend must never `parseFloat()` a money field for anything other than display formatting through a dedicated, tested money-handling utility — this is a direct, non-negotiable carry-forward of the backend's `numeric(18,4)` discipline (`08_database_design.md` §20).
3. **Multi-Currency Is Never Silently Blended.** Per `08_database_design.md` §14, Analytics data is currency-partitioned, not auto-converted. The frontend must render multi-currency data as genuinely separate totals, never summed across currencies — a direct UI-layer consequence of a backend invariant.
4. **Confidence and Provenance Are Visible, Not Hidden.** Per the Vision's "Trust Through Auditability" principle (`01_product_vision.md` §8), the frontend surfaces extraction confidence and correction history wherever a user might reasonably ask "why does the system think this?" — trust-through-transparency is a frontend responsibility as much as a backend one.
5. **Pending-Review Is a First-Class State, Not an Edge Case.** Because a meaningful fraction of events may require user confirmation (`04_domain_model.md`'s `pending_review` status), the frontend's information architecture treats this state as a primary, well-designed flow — not a buried notification badge.
6. **Degraded Connectivity Degrades Gracefully, Never Silently.** Per the PWA scope decision (Section 5), the frontend must behave predictably offline or on a flaky connection — showing clearly-labeled stale data rather than either blocking entirely or silently showing wrong data.
7. **Accessibility and Internationalization Are Launch Requirements, Not Polish.** A financial product serving a diverse user base cannot treat these as optional — formalized in `37_accessibility_i18n.md`, established here as non-negotiable scope.

---

## 7. Detailed Explanation: Why the Frontend Gets Its Own 21-Document Series

It might seem excessive to apply the same documentation rigor used for a modular-monolith Go backend with eight bounded contexts to what is, structurally, "one React app." This series rejects that framing for the same reason the backend series rejected "it's just an expense tracker":

1. **The frontend's complexity is not in its technology, but in its domain fidelity.** Correctly rendering a `FinancialEvent`'s full lifecycle (14 event types, confidence-gated states, override history, multi-currency aggregates, PWA offline states) faithfully to the backend's actual guarantees is a genuinely nontrivial design problem — one that, left unspecified, would very likely drift from the backend's carefully-designed invariants (e.g., a frontend that casually sums two currencies in a "total spend" widget would silently violate a guarantee the backend went to considerable lengths to protect).
2. **A frontend built without an equivalent information-architecture and state-management specification is where well-designed backends go to have their guarantees quietly undone at the last mile** — this is a common, well-documented failure mode in software systems generally, and this series exists specifically to prevent it here.
3. **The sprint checklists already proved the need.** Section 3.1's point: the frontend tasks were already dense and specific enough, spread across twelve sprints, that assembling them into a single coherent specification — rather than leaving each sprint's frontend section as an island — produces a genuinely more buildable, more consistent result, exactly as the backend series' Documents 02–24 did for the backend sprint work.

---

## 8. Component Responsibilities (Series-Level)

| Document | Responsibility |
|---|---|
| `26_frontend_product_requirement_document.md` | Functional requirements, user stories, acceptance criteria — the frontend's own `FR-*` catalog |
| `27_frontend_software_requirement_specification.md` | Engineering-grade NFRs: performance budgets, browser support matrix, offline requirements |
| `28_frontend_information_architecture.md` | Full screen/route map, navigation model, domain-to-UI mapping |
| `29_frontend_architecture.md` | Layering, data flow, rendering strategy, folder-level architecture |
| `30_design_system.md` | Visual language, tokens, base component set |
| `31_screen_specifications.md` | Every screen, implementation-ready, consolidating all sprint-checklist frontend tasks |
| `32_component_library.md` | Reusable component catalog (forms, tables, charts, money display) |
| `33_state_management_data_fetching.md` | Client state, server-state caching, optimistic updates |
| `34_api_integration_layer.md` | The concrete client-side implementation of `09_api_specification.md`'s contract |
| `35_routing_navigation.md` | Route structure, guards, deep linking |
| `36_pwa_specification.md` | Manifest, service worker, offline/caching strategy |
| `37_accessibility_i18n.md` | A11y standards, localization/currency-formatting strategy |
| `38_frontend_testing_strategy.md` | Component, integration, E2E testing |
| `39_frontend_security.md` | XSS/CSRF posture, token storage, CSP |
| `40_frontend_performance_observability.md` | Core Web Vitals, bundle budgets, RUM/error tracking |
| `41_frontend_deployment.md` | Build pipeline, hosting, environment config |
| `42_frontend_coding_convention.md` | Naming, component patterns, review checklist |
| `43_frontend_project_structure.md` | Concrete repository file tree |
| `44_frontend_risk_analysis.md` | Frontend-specific risk register |
| `45_frontend_future_roadmap.md` | Post-MVP frontend evolution |

---

## 9. Trade-offs

| Decision | Trade-off |
|---|---|
| PWA, not native app, at MVP | Forgoes native push notifications and deeper OS integration, in exchange for a single codebase, faster iteration, and no app-store distribution overhead — acceptable because WhatsApp already owns the proactive-notification role (Section 5.2). |
| Full documentation rigor for the frontend, matching the backend series | Significant upfront design investment before UI code is written, in exchange for preventing the "well-specified backend, ad hoc frontend" drift risk described in Section 7. |
| React + Vite over a meta-framework (e.g., Next.js) | Forgoes built-in SSR/file-based routing conveniences, in exchange for a simpler, more predictable client-rendered architecture appropriate for an authenticated, largely-personal-dashboard product where SEO/SSR has little value — this trade-off is revisited explicitly in `29_frontend_architecture.md`. |

## 10. Advantages
- Establishes the frontend as a first-class, intentionally-designed product surface from the outset, rather than an implementation detail assembled ad hoc from scattered sprint-checklist notes.
- Directly inherits and extends the backend series' hard-won principles (decimal money, currency partitioning, confidence transparency) rather than risking their silent erosion at the UI layer.
- Gives the sprint-checklist series' frontend tasks a stable, authoritative specification to build against, closing the gap identified by Iqbal between "tasks exist" and "tasks are actually specified."

## 11. Disadvantages
- A 21-document frontend series is a substantial additional authoring and review investment on top of the already-extensive backend series — a real cost, accepted here because the user explicitly requested matching rigor.
- Some of this series' guidance (e.g., exact component library choices) will necessarily involve more subjective design judgment than the backend series' correctness-driven decisions (e.g., INV-3's uniqueness constraint) — readers should expect slightly more "reasonable choice, open to revision" framing in design-system-adjacent documents than in, say, `34_api_integration_layer.md`'s contract-adherence rules.

## 12. Future Improvements
- Revisit the "PWA, not native" decision (Section 5) if user research or engagement data post-launch suggests native push notifications or deeper OS integration would meaningfully improve retention beyond what WhatsApp already provides.
- Consider Next.js or a similar meta-framework if the product ever needs public, SEO-relevant, unauthenticated pages (e.g., a marketing site) that would benefit from SSR — currently out of scope, since the entire frontend sits behind authentication.

## 13. Best Practices
- Every frontend document in this series must cite the backend document/section it depends on or extends, exactly as the backend series cited across its own documents — maintaining one coherent, cross-referenced specification body rather than two disconnected series.
- No frontend document may introduce a data shape, field name, or business rule not already present in `09_api_specification.md` or the backend domain model — if a UI need reveals a gap in the API contract, that gap is raised back against the backend series, not silently worked around in the frontend.

## 14. Production Considerations
- This series' guidance must be revisited whenever `09_api_specification.md` changes — the frontend series is downstream of and dependent on the API contract staying the source of truth; a frontend-only change to, say, a money-formatting convention should never diverge from what the backend actually sends.
- As with the backend series' `20_coding_convention.md`, the eventual frontend PR review process should treat this series' conventions as enforced, not aspirational, from the first frontend commit.

---

**Next Document:** `26_frontend_product_requirement_document.md` — translates this vision into concrete frontend functional requirements and user stories, consolidating every screen/flow already named across the Sprint 1–9 "Frontend (Parallel Track)" checklists into a formal `FE-FR-*` requirement catalog.
