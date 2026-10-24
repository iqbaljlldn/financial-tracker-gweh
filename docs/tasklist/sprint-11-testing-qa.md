# Sprint 11 — Testing & QA Hardening

**Theme:** Close every testing gap across unit, integration, contract, AI/template evaluation, and E2E layers. Includes frontend test coverage, which has not had a dedicated sprint until now.

**Primary documents:** `18_testing_strategy.md` (all)

**Entry criteria:** Sprints 1–10 complete.

---

## Checklist

### Coverage Gates (Backend)
- [ ] Domain layer unit coverage ≥90% across all modules, verified in CI, not just `financialcore` (`18_testing_strategy.md` §5.3) **[GATE]**
- [ ] Application layer unit coverage ≥80%
- [ ] Adapter layer unit coverage ≥60%
- [ ] All five `04_domain_model.md` invariants (INV-1–5) confirmed to have named, passing tests (re-verify from Sprint 6, don't just assume it held)

### Integration — Full Diagram Coverage
- [ ] All eight `07_sequence_diagrams.md` scenarios confirmed covered by an integration test, including documented failure branches (`18_testing_strategy.md` §6.2) — cross-check against the table in that section, item by item **[GATE]**
- [ ] Diagram 5 (duplicate delivery) and Diagram 8 (purge cascade) specifically re-verified, as the two highest-stakes flows
- [ ] Cross-module boundary tests confirming every sanctioned interface (`10_backend_blueprint.md` §6) works end to end with real Postgres

### Contract Testing
- [ ] OpenAPI-schema-based contract tests covering every endpoint in `09_api_specification.md`'s full catalog (`18_testing_strategy.md` §7.1) **[GATE]**
- [ ] Recorded-fixture contract tests for Gmail, WhatsApp, and LLM provider client code (`18_testing_strategy.md` §7.2)

### AI & Template Evaluation Suites
- [ ] AI evaluation set finalized with representative fixtures across all 14 `event_type`s, multiple currencies, ambiguous cases (`18_testing_strategy.md` §8.1)
- [ ] Baseline accuracy/calibration metrics recorded for the launch prompt version — this becomes the regression reference point going forward **[GATE]**
- [ ] Template sample-set suites confirmed passing for every MVP-curated sender

### E2E
- [ ] Full onboarding-to-first-notification E2E flow (`18_testing_strategy.md` §9)
- [ ] GDPR purge-cascade E2E flow, invoked through the actual public API exactly as a real client would (`18_testing_strategy.md` §12) **[GATE — this is deliberately redundant with the Sprint 10 integration test; both must pass]**

### Frontend Testing (New This Sprint)
- [ ] Component/unit tests for core UI components (API client wrapper, money formatting, error envelope display)
- [ ] E2E browser tests (e.g., Playwright) for the critical user journeys: sign in → connect Gmail → see synced transactions → create a budget → view dashboard → confirm a pending-review item
- [ ] Frontend accessibility pass (basic keyboard nav, contrast, form labeling) — not exhaustive, but not skipped either
- [ ] Cross-browser smoke check (at minimum: Chrome + Safari, given WhatsApp/mobile-adjacent usage patterns)

### CI Pipeline Closure
- [ ] Every stage from `18_testing_strategy.md` §10's table confirmed actually blocking (not a stub/no-op left over from Sprint 0) **[GATE]**
- [ ] Frontend build/test/lint added as its own CI stage, blocking on the same branch-protection rules as backend

### Testing-of-the-Tests
- [ ] Deliberately break one domain invariant in a throwaway branch, confirm the corresponding unit test actually fails (catches a "test that doesn't test anything" false-positive)
- [ ] Deliberately introduce a forbidden cross-module import, confirm the boundary linter blocks it

---

## Exit Criteria

- [ ] Full CI pipeline (backend + frontend) is green on a representative release-candidate branch
- [ ] Every sequence-diagram scenario and every invariant has a verified, passing, named test
- [ ] AI evaluation baseline is documented and will be used as the regression reference for every future prompt change
