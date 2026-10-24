# Sprint 12 — Deployment & Launch Readiness

**Theme:** Full deployment pipeline (backend + frontend), staging soak, operational runbook sign-off, and the consolidated production-readiness checklist. This is the last sprint before launch.

**Primary documents:** `19_deployment.md` (all), `22_risk_analysis.md`, `23_scalability.md`

**Entry criteria:** Sprints 1–11 complete.

---

## Checklist

### Container Builds
- [ ] Backend multi-stage Containerfile finalized: minimal runtime image, SHA-tagged, no `latest` used as a deploy target (`19_deployment.md` §4) **[GATE]**
- [ ] Frontend build: static assets built via Vite, served via its own minimal container (e.g., nginx/Caddy) or a CDN/static-hosting target — decide and document which, since this wasn't specified in the original backend-focused series
- [ ] Frontend container/static deployment wired into Traefik routing (same TLS termination point as the API, or a separate CDN domain — document the choice)

### Environment Topology
- [ ] dev / staging / production environments stood up per `19_deployment.md` §3
- [ ] Staging uses synthetic/anonymized data only — confirmed no real user data ever present there (`18_testing_strategy.md` §11, `19_deployment.md` §3)
- [ ] Frontend environment-specific config (API base URL, feature flags) correctly wired per environment, no hardcoded staging URLs in a production build

### CI/CD Pipeline — Full Chain
- [ ] Full pipeline live: lint → unit → integration → contract → ai-eval/template-eval → vuln-scan → build-image → staging deploy → smoke-test → **manual approval gate** → production deploy → smoke-test → monitor window (`19_deployment.md` §5) **[GATE]**
- [ ] Frontend pipeline mirrors the same gating discipline (build → test → deploy to staging → manual approval → deploy to production)
- [ ] Database migrations confirmed running as their own pipeline stage, before code rollout, with backward-compatibility review documented per migration (`19_deployment.md` §6) **[GATE]**

### Release Strategy
- [ ] Rolling deployment verified for API instances (zero-downtime)
- [ ] Graceful worker shutdown verified (in-flight Asynq tasks complete before termination)
- [ ] Rollback drill actually performed in staging — not just documented as possible (`19_deployment.md` §17) **[GATE]**
- [ ] Emergency patch procedure drilled once in staging before it's ever needed for real (`19_deployment.md` §9, §18)

### Operational Runbooks
- [ ] Every failure domain in `06_architecture.md` §12 (as extended by Sprint 10's KMS addition) has a corresponding runbook entry in `19_deployment.md` §8, reviewed and acknowledged by whoever is on-call
- [ ] On-call rotation established

### Capacity & Quota Planning
- [ ] Gmail API quota headroom checked against projected launch-scale usage, increase request filed if the 70% planning trigger is already close (`23_scalability.md` §6.1)
- [ ] WhatsApp messaging tier checked against projected notification volume (`23_scalability.md` §6.2)
- [ ] LLM provider rate limit checked against projected AI-extraction volume (`23_scalability.md` §6.3)

### Production Readiness Checklist (Final Gate) — from `19_deployment.md` §10
- [ ] Every blocking CI stage green on the release candidate
- [ ] Full metric/dashboard catalog populated from a real staging soak period
- [ ] GDPR purge-cascade dry-run completed and **observed**, not merely automated-tested
- [ ] KMS failure-domain entry closed (carried from Sprint 10)
- [ ] All WhatsApp templates approved for every launch-scope `notification_type`
- [ ] Known-sender template library covers the MVP curated sender list, every template passing sample-set validation
- [ ] AI evaluation-set baseline established and documented
- [ ] Secrets rotation schedule calendared
- [ ] Quota/tier headroom verified
- [ ] On-call team has reviewed and acknowledged runbooks
- [ ] **[NEW — frontend-specific]** Frontend production build verified against the production API contract (no stale mock data, no dev-only shortcuts shipped)
- [ ] **[NEW — frontend-specific]** Frontend error tracking and basic usage analytics confirmed live in production before real users land on it

### Launch
- [ ] Production deploy executed through the full gated pipeline
- [ ] Monitor window observed actively (`19_deployment.md` §5.2) by on-call, watching the Platform Overview dashboard
- [ ] First real user's onboarding flow watched end-to-end by someone on the team (don't just trust the dashboards on day one)

---

## Exit Criteria

- [ ] Platform is live in production, serving real users
- [ ] Every item in the Production Readiness Checklist above is checked — no exceptions deferred silently
- [ ] First 24 hours post-launch pass without a Critical-severity incident

---

## Immediately After This Sprint

Proceed to `24_future_roadmap.md` §4's mandatory post-launch calibration work (confidence threshold tuning using real data, risk-register re-scoring, burn-rate alerting setup, template-library expansion from real traffic) — this is not optional follow-up, it was explicitly deferred to this exact point throughout the series.
