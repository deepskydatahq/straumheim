# Zero-ops hosting requirements

Status: decision baseline for M008, reviewed 2026-08-20.

## Meaning of “zero ops”

For this mission, zero ops means **no guest OS, reverse proxy, or container orchestrator to patch; no SSH-based recovery; and no recurring manual uptime check**. It does not mean zero responsibility. The operator still owns configuration, credentials, destination availability, application releases, incident response, and the semantics of accepted events.

A platform must pass every mandatory gate before its weighted score matters.

## Mandatory gates

| Gate | Required evidence |
|---|---|
| Managed substrate | Provider patches and replaces host/orchestrator infrastructure; no VM login is part of normal operation. |
| Continuous health recovery | Running instances are checked continuously; an unresponsive process is removed from traffic and restarted automatically. Crash-only restart is insufficient. |
| Safe deployment | A candidate must become healthy before replacing the prior healthy deployment. A failed candidate leaves the prior version serving. |
| Managed ingress | Public HTTPS and certificate renewal require no Caddy/Nginx instance. Custom domain support is available for production. |
| EU placement | Collector compute can run in an EU region. |
| Background execution | Straumheim’s batch goroutine receives CPU outside request handling for the full instance lifetime. |
| Central evidence | Runtime logs and platform events persist outside the instance. |
| Actionable notification | Failed deploys and unhealthy running services can notify by email, Slack, or webhook without a self-hosted monitor. |
| Cost ceiling | Low-volume always-available collector compute is at most EUR 20/month, excluding destination DB, domain, taxes, and material egress. |
| Alert-driven operation | No scheduled SSH, package update, restart, or uptime checklist is needed. |

## Weighted criteria

Scores use 0–5 per category. Weighted points equal `score / 5 × weight`.

| Category | Weight | What earns 5/5 |
|---|---:|---|
| Infrastructure automation and recovery | 25 | Continuous application-level health checks, traffic removal, restart, managed host patching. |
| Deployment safety and rollback | 15 | Health-gated zero-downtime replacement plus fast immutable rollback. |
| Straumheim runtime fit | 20 | Always-on/background CPU, graceful SIGTERM window, outbound DB access, no persistent-local-disk assumption. |
| Observability and notifications | 15 | Central logs and native unhealthy/deploy-failure notifications on the affordable plan. |
| Cost predictability | 15 | Clearly below the ceiling for one always-on 512 MiB instance without fragile idle assumptions. |
| EU, security, and portability | 10 | EU region, managed TLS/secrets, standard container workflow, limited lock-in. |

## Responsibility boundary

| Responsibility | Managed platform | Straumheim/operator |
|---|---|---|
| Host OS, runtime hosts, failed hardware | Owns patching and replacement | Verify provider status only during incidents |
| TLS termination and renewal | Owns | Own DNS records and domain registration |
| Process liveness | Probe, remove traffic, restart | Expose a meaningful endpoint and avoid crash loops |
| Deployment gating | Keep old version until candidate passes | Publish immutable source/image and run CI |
| Rollback mechanism | Retain/redeploy prior artifact | Choose rollback trigger and validate data path |
| Runtime and platform logs | Capture and retain per plan | Emit useful structured events and define alert policy |
| Destination credentials | Encrypt/store supplied values | Rotate values and grant least privilege |
| Destination health | Cannot infer application semantics | Expose readiness/delivery state and respond to alerts |
| Accepted-event durability | Cannot preserve process memory | Implement durable buffering, retries, and idempotency |
| Cost control | Metering and billing alerts | Set budget threshold and review only on alert/change |

## Runtime audit against the repository

1. **Background work requires continuous CPU.** `cmd/straumheim/main.go:71` creates an in-memory buffer. Its consumer starts a goroutine (`internal/buffer/memory.go:55`) and flushes on a timer (`internal/buffer/memory.go:58-83`) after the HTTP response can already be complete. A platform that allocates CPU only while requests run is unsafe without changing Straumheim.
2. **Restarts can lose acknowledged records.** Records live in a channel created by `NewMemoryBuffer` (`internal/buffer/memory.go:28`). Graceful cancellation drains it (`internal/buffer/memory.go:84-101`), but process crashes, SIGKILL, platform faults, and a shutdown deadline can discard queued records.
3. **Sink failures are not retried.** The engine records failure metrics and logs the error (`internal/pipeline/engine.go:61-71`) then proceeds; it does not requeue the batch. Hosting recovery does not provide event-delivery recovery.
4. **Health is shallow.** `/health` is registered at `cmd/straumheim/main.go:107`; `healthHandler` at line 166 always returns `{"status":"ok"}` and does not inspect the buffer or sinks. It is valid for process liveness, not production readiness.
5. **Graceful shutdown exists but is bounded.** SIGTERM/SIGINT are handled at `cmd/straumheim/main.go:123`; HTTP shutdown receives 10 seconds (`cmd/straumheim/main.go:148`) before the engine drains (`cmd/straumheim/main.go:157`). The host must allow at least that window, and a large/blocked flush can exceed it.
6. **The container is portable and minimal.** The runtime is `scratch` (`Dockerfile:12`), listens on configured `0.0.0.0:8080`, and exposes 8080 (`Dockerfile:17`). No shell or host agent should be assumed.
7. **Local file output is incompatible with ephemeral roots.** The file sink requires `output_dir` and writes locally (`cmd/straumheim/main.go:197-204`). The hosting proof must use stdout-to-platform-logs or an external database, not local persistence.
8. **Port injection needs explicit handling.** Straumheim reads its YAML port rather than a provider `PORT` variable. The proof must pin the platform target port to 8080 or supply a matching config.

## Proof success rules

A live proof is successful only when evidence shows:

- HTTPS `/health` succeeds in an EU region;
- a webhook response ID is also visible in the configured non-production destination;
- an unhealthy candidate never replaces the healthy service;
- a running health failure is detected, removed, restarted, and notified;
- a prior healthy artifact is restorable;
- logs and one real notification are captured; and
- the observed shutdown/restart window is documented without claiming zero event loss.

Until these observations exist, a platform decision can be **selected for proof**, but production migration remains blocked.
