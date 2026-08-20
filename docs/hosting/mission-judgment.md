# M008 product judgment

Judgment date: 2026-08-20

Result: **blocked — 8 criteria pass, 6 require live Render evidence**

The platform decision is usable for a proof, not yet approved for production migration.

| # | Mission criterion | Result | Evidence |
|---:|---|---|---|
| 1 | Requirements distinguish platform/application ownership | Pass | `requirements.md`: mandatory gates, responsibility boundary, runtime audit |
| 2 | Preferred platforms scored from official evidence | Pass | `platform-comparison.md`: one weighted matrix and linked official sources |
| 3 | DigitalOcean excluded unless necessary | Pass | Render passes all gates; exclusion is documented in comparison and decision |
| 4 | EU always-available cost and exclusions | Pass | Render $7; Cloud Run $44.71 safe estimate; Railway $6–10 range with assumptions |
| 5 | Live immutable deployment with secrets, TLS, health | **Blocked** | Blueprint/proof assets exist, but no Render workspace was available |
| 6 | Synthetic HTTPS event reaches non-production sink | **Blocked** | Requires live service and Render logs |
| 7 | Unhealthy candidate does not replace healthy deploy | **Blocked** | Drill defined in `proof-plan.md`, not observed |
| 8 | Unhealthy/stopped instance automatically recovers | **Blocked** | Official behavior documented; mission requires observed proof |
| 9 | Central logs and tested failure notification | **Blocked** | Notification destination/workspace unavailable |
| 10 | No recurring manual uptime check | Pass | `requirements.md` definition and alert-driven operations in `migration-runbook.md` |
| 11 | Restart/shutdown and event-loss implications | **Blocked** | Repository audit and proof plan explain SIGTERM, timer flush, and memory-loss windows, but the criterion requires observed restart/shutdown timing |
| 12 | Cloud Run background CPU/billing addressed | Pass | `platform-comparison.md` calculates instance-based minimum cost and rejects request-based CPU |
| 13 | Production migration plan | Pass | `migration-runbook.md` covers secrets, preflight, canaries, DNS, rollback, observation, and Hetzner retirement |
| 14 | Reliability gaps represented in product TOML | Pass | Draft M009 durable delivery and M010 operational health missions |

## Completed artifacts

- `requirements.md`
- `platform-comparison.md`
- `decision.md`
- `proof-plan.md`
- `proof-results.md` (truthful blocked record)
- `migration-runbook.md`
- `render.yaml`
- `deploy/render/config.proof.example.yaml`
- M009 and M010 draft missions

## Unblock condition

Provide access to a Render workspace that can create a temporary Starter service in Frankfurt and configure email or Slack notifications. Execute M008-E002-S002 and S003, replace the blocked proof-results section with observed evidence, then repeat product judgment. No production credentials or DNS changes are required for the baseline proof.
