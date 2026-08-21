# Hosting decision: Render Starter in Frankfurt

- **Status:** validated by disposable Free proof; paid Starter smoke test and production cutover approval remain
- **Date:** 2026-08-20
- **Mission:** M008

## Decision

Use a **Render web service on the Starter plan in Frankfurt**, deployed as a Git-backed Docker service from this repository. Configure:

- `autoDeployTrigger: checksPass` so only commits with successful GitHub checks deploy;
- `healthCheckPath: /health` for candidate gating and continuous process recovery;
- two Starter instances (512 MB and 0.5 CPU each) with no persistent disk, so one healthy peer can serve while Render replaces the other;
- `maxShutdownDelaySeconds: 30`, matching Straumheim’s 10-second HTTP shutdown budget with platform margin;
- `STRAUMHEIM_CONFIG=/etc/secrets/config.yaml` and a Render-managed `config.yaml` secret file;
- platform HTTPS and, after proof, a custom collector domain;
- failure notifications by email and optionally Slack;
- stdout as proof sink, persisted in Render logs; production uses an external Postgres/ClickHouse sink.

The expected collector compute cost is **$14/month** (2 × $7 Starter), excluding destination database, bandwidth, domain, taxes, and currency conversion. This remains below M008's EUR 20/month target before currency/tax variation.

## Why Render

Render is the only preferred candidate that combines all of the following below the mission’s cost ceiling:

1. continuous application health requests, traffic removal, restart, and unhealthy-service notification;
2. two-instance operation so a healthy peer can continue serving during replacement;
3. health-gated zero-downtime deployment that leaves the old revision serving when a candidate fails;
4. always-running CPU suitable for Straumheim’s timer-driven batch goroutine;
5. Frankfurt placement, managed TLS, secret files, external logs, and rollback; and
6. a simple source-controlled Blueprint without a VM, reverse proxy, or monitor to maintain.

## Rejected alternatives

- **Cloud Run:** technically strong, but safe instance-based CPU with one minimum 1-vCPU/512-MiB instance is estimated at $44.71/month after the documented free tier. Request-based CPU is incompatible with background timer flushes.
- **Railway:** expected around $6–10/month, but Railway explicitly does not monitor the configured health endpoint after deployment. Crash restart does not recover a live deadlock. Native resource monitors require the $20 Pro plan and do not close that gap.
- **DigitalOcean App Platform:** not evaluated because Render passes all mandatory gates and the owner requested DigitalOcean only as a last resort.
- **Hetzner VM/Coolify:** require host, patch, backup, orchestrator, and monitoring ownership and therefore contradict zero-ops requirements.

## Deployment choice: source build versus GHCR

Use a **Git-backed Docker service** for the proof and initial production plan:

- Render builds the repository Dockerfile from the exact commit;
- auto-deploy can wait for GitHub CI;
- each successful deploy has a retained artifact suitable for rollback; and
- there is no mutable `latest` tag race or deploy-hook secret.

The public GHCR image remains the portable distribution artifact. If production later switches to an image-backed Render service, pin an OCI digest—not `latest`—and have CI trigger the Render deploy hook explicitly after publishing.

## What this decision does not guarantee

- `/health` currently says only that the process responds; it does not test destinations.
- Render restart or deploy can discard records held in the affected instance's in-memory buffer. Two instances improve endpoint availability but do not replicate queued records or make delivery durable.
- Failed sink writes are logged and counted but not retried.
- The one-instance Free proof observed approximately 52 seconds of HTTP 502 during automatic replacement. Production therefore uses two Starter instances so a healthy peer remains available. Each instance must be sized to carry full current traffic during peer recovery.
- Render’s platform automation does not make accepted events durable.

These are application roadmap items captured in M009 and M010, not reasons to retain a VM.

## Production approval gates

Do not cut over DNS until all are true:

- the live proof stories M008-E002-S002 and M008-E002-S003 are complete;
- synthetic event ID and stdout/debug delivery are observed in Render logs;
- unhealthy candidate, running health failure, restart, rollback, SIGTERM, and notification behavior are recorded;
- production destination connectivity is validated with least-privilege credentials;
- delivery-failure alerting has an interim or application-native solution; and
- rollback ownership and DNS procedure are approved.

See [platform-comparison.md](platform-comparison.md), [proof-plan.md](proof-plan.md), and [migration-runbook.md](migration-runbook.md).
