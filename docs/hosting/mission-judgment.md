# M008 product judgment

Judgment date: 2026-08-21

Result: **complete — all 14 criteria pass**

A live Frankfurt Free service validated Render's platform behavior without adding payment information. The committed production recommendation remains Starter; Free cannot validate the explicit 30-second shutdown-delay setting and spins down when idle.

| # | Mission criterion | Result | Evidence |
|---:|---|---|---|
| 1 | Requirements distinguish platform/application ownership | Pass | `requirements.md`: mandatory gates, responsibility boundary, runtime audit |
| 2 | Preferred platforms scored from official evidence | Pass | `platform-comparison.md`: one weighted matrix and linked official sources |
| 3 | DigitalOcean excluded unless necessary | Pass | Render passes all gates; exclusion is documented in comparison and decision |
| 4 | EU always-available cost and exclusions | Pass | Render $7; Cloud Run $44.71 safe always-active estimate; Railway $6–10 range with assumptions |
| 5 | Live immutable deployment with secrets, TLS, health | Pass | Frankfurt service built exact commit `d6a16cd`; secret-file config, verified TLS, and `/health` passed |
| 6 | Synthetic HTTPS event reaches non-production sink | Pass | Event ID `01a022aa-5a3f-7cd3-a634-2120dcad8e72` matched the stdout record in Render logs |
| 7 | Unhealthy candidate does not replace healthy deploy | Pass | Candidate `763c988` failed after the health window while the previous revision stayed HTTP 200 and accepted a new event |
| 8 | Unhealthy/stopped instance automatically recovers | Pass | Delayed failure produced 503, traffic removal/502, SIGTERM, replacement, then 200 without SSH |
| 9 | Central logs and tested failure notification | Pass | Central logs captured delivery and lifecycle evidence; the operator confirmed receipt of Render's failure email |
| 10 | No recurring manual uptime check | Pass | `requirements.md` definition and alert-driven operations in `migration-runbook.md` |
| 11 | Restart/shutdown and event-loss implications | Pass | Logs show shutdown signal, pipeline flush, and completion; proof records a roughly 52-second single-instance interruption and memory-loss caveat |
| 12 | Cloud Run background CPU/billing addressed | Pass | `platform-comparison.md` calculates the safe always-active cost and rejects request-based CPU for the current goroutine model |
| 13 | Production migration plan | Pass | `migration-runbook.md` covers secrets, preflight, canaries, DNS, rollback, observation, and Hetzner retirement |
| 14 | Reliability gaps represented in product TOML | Pass | Draft M009 durable delivery and M010 operational health missions |

## Important observed behavior

- Baseline Docker deployment took about 54 seconds.
- Running health failure was removed from traffic after roughly 15 seconds.
- A single instance returned 502 for roughly 52 seconds while Render replaced it.
- An unhealthy candidate took about 15 minutes to be marked `update_failed`, but the healthy revision served throughout.
- API rollback restored the prior commit in about 22 seconds while health remained 200.
- Graceful shutdown completed for the fast stdout proof sink; this does not guarantee a blocked database sink drains in time.

## Completed artifacts

- `requirements.md`
- `platform-comparison.md`
- `decision.md`
- `proof-plan.md`
- `proof-results.md`
- `migration-runbook.md`
- `render.yaml`
- `deploy/render/config.proof.example.yaml`
- M009 and M010 draft missions

## Completion

The operator confirmed notification receipt. The disposable service and temporary proof branches were deleted, and the workspace has no remaining Render services. A paid Starter smoke test remains recommended before production cutover but is not required to validate the shared health, deployment, rollback, logging, and notification mechanisms.
