# Sprint 10 — Security & Observability Hardening (Full Closure)

**Theme:** Close every security and observability item that was deferred, stubbed, or partially built across Sprints 1–9. This sprint does not introduce new domain features — it makes everything already built production-safe and fully visible.

**Primary documents:** `16_security.md` (all), `17_observability.md` (all)

**Entry criteria:** Sprints 1–9 complete.

---

## Checklist

### Encryption & Key Management (Verify, Don't Assume)
- [ ] KMS envelope encryption verified across **every** token field, not just spot-checked (`16_security.md` §4.3)
- [ ] Key rotation procedure actually exercised once in staging, not just documented (`16_security.md` §4.4)
- [ ] **[GATE — closes R-V6 from `22_risk_analysis.md`]** KMS failure explicitly added to the failure-domain table (`06_architecture.md` §12) and given a runbook entry (`19_deployment.md` §8) — this was the series' single flagged open item

### RBAC & Admin Surface
- [ ] `/admin/*` endpoints fully implemented: `GET /admin/connectors/health`, `/parsers/metrics`, `/prompts/metrics`, `/audit-log` (`09_api_specification.md` §6.6)
- [ ] `ops` role provisioning verified individual, non-shared, audit-logged on every access (`16_security.md` §6.3)
- [ ] Row-level ownership checks spot-audited across every query handler written in Sprints 1–9 (`16_security.md` §6.2) **[GATE]**

### PII & Data Classification
- [ ] Full Tier 1–4 classification review (`16_security.md` §7) across every log line, span attribute, and LLM prompt written so far — no plaintext PII found in any of the three
- [ ] GDPR data export endpoint: `GET /me/data-export` (`16_security.md` §9.1)
- [ ] GDPR purge cascade fully implemented end to end (Diagram 8): disconnect → archive → analytics exclusion → audit-exempt logging → scheduled permanent-erasure sweep **[GATE — the platform's single highest-stakes flow]**

### Secrets & Vulnerability Management
- [ ] All runtime secrets confirmed sourced from secrets manager, zero in version control (`16_security.md` §10)
- [ ] Dependency and container-image vulnerability scanning confirmed blocking in CI, not just present (`16_security.md` §11)

### n8n Governance
- [ ] n8n instance stood up, scoped strictly per `14_workflow_engine.md` §5 (read-only, narrowly-credentialed, no direct DB/Redis network path)
- [ ] At minimum: Prometheus-alert-to-Slack routing workflow built as the first sanctioned use case

### Observability — Full Catalog Closure
- [ ] Every metric in `17_observability.md` §4's full catalog confirmed present and populated (cross-check against each sprint's partial list) **[GATE]**
- [ ] Full Grafana dashboard catalog stood up: Platform Overview, Connector Health, Parser Monitoring, Prompt Monitoring, Financial Core Health, Analytics Freshness, Engagement/Notification, Security & Compliance, Cost & Capacity (`17_observability.md` §7)
- [ ] Every alert rule from `17_observability.md` §8 configured and routed (via n8n) with correct severity
- [ ] SLO dashboard stood up for every target in `17_observability.md` §9
- [ ] End-to-end trace propagation verified across at least one full flow: Gmail webhook → ... → WhatsApp delivery, confirmed as one coherent trace, not fragmented per hop **[GATE]**

### Frontend (Security & Observability-Adjacent)
- [ ] Frontend error/exception tracking confirmed capturing real errors (not just wired, actually verified with a deliberate test error)
- [ ] Frontend session-expiry handling verified graceful (no silent failures, no leaked tokens in browser storage beyond what's necessary)
- [ ] Frontend: confirm no PII is logged to browser console or sent to any third-party analytics script
- [ ] Admin-only screens (if any were built as a stopgap in Sprints 4–5) now properly gated behind the real `ops` role check, or removed in favor of the real `/admin` API

### Testing
- [ ] Full `16_security.md` §12's RLS-vs-application-enforcement reasoning spot-verified with a deliberate "forgot the WHERE clause" test (confirm it's caught by the Section 6.2 ownership-parameter convention, not silently passing)
- [ ] GDPR cascade integration test (Diagram 8) passing, including the audit-log-survives-erasure assertion **[GATE]**

---

## Exit Criteria

- [ ] Zero open items remain from `22_risk_analysis.md`'s register that were marked "Open" or "requires action before launch"
- [ ] Every dashboard in `17_observability.md` §7 is live and populated with real staging data
- [ ] A deliberately-triggered test incident (e.g., kill Redis in staging) produces the expected alert, routed correctly, within the expected time
