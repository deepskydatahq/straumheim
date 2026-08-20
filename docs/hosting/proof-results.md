# Render proof results

- **Status:** blocked before live deployment
- **Last attempted:** 2026-08-20
- **Selected platform:** Render Starter, Frankfurt

## Access check

| Dependency | Result |
|---|---|
| Public multi-architecture GHCR image | Pass — `ghcr.io/deepskydatahq/straumheim:latest` exposes `linux/amd64` and `linux/arm64` manifests |
| GitHub repository access | Pass — authenticated GitHub CLI can access the repository |
| Render CLI/API credentials | **Blocked — no Render CLI, API token, service, or authenticated workspace is available in this environment** |
| Disposable Render billing | **Blocked — cannot create a Starter service without workspace access** |
| Notification destination | **Blocked — no Render email/Slack workspace can be configured from this environment** |
| Disposable external database sink | Not available; the planned baseline uses stdout persisted in Render logs |

A Google Cloud CLI is installed, but its active credentials require interactive reauthentication. Cloud Run was also rejected by the cost/runtime analysis, so using an unrelated Google project would not validate the selected Render setup.

## Local asset validation

The Docker build completed successfully after adding `.dockerignore`; local build context dropped from 372.2 MB to 1.055 MB. Running the built scratch image with the planned secret-file mount and command override produced:

- `GET /health` → `{"status":"ok"}`
- `POST /webhook` with `proof_id=m008-local-20260820` → event ID `01a020f1-98c2-7415-bb89-8d413a5732e7`
- the stdout sink emitted a JSON record containing the same event ID and proof ID

This validates the image, config path, input, buffer flush, and proof sink locally. It does not validate Render.

## Live evidence

Not executed. No Render service URL, deployment event, webhook event, restart, rollback, or notification result is claimed.

| Drill | Status | Evidence |
|---|---|---|
| Frankfurt HTTPS health | Blocked | Render workspace required |
| Synthetic event to persisted debug logs | Blocked | Live service/log access required |
| Unhealthy candidate retains healthy deploy | Blocked | Live service required |
| Running health failure removes/restarts instance | Blocked | Live service and disposable instrumented revision required |
| Failure notification delivery | Blocked | Render notification destination required |
| Rollback and SIGTERM observation | Blocked | Two successful live deploys required |
| External sink outage visibility | Blocked | Disposable destination credentials required |

## Unblock instructions

Provide an authenticated Render workspace with permission to create a temporary Starter service and configure email or Slack notifications. Then follow `proof-plan.md`. No production secret or DNS access is required.

After the proof, replace this blocked section with observed UTC timestamps, non-secret commands, deployment identities, response/event IDs, log excerpts, notification outcome, timing, and cleanup confirmation.
