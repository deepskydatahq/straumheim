# Managed hosting comparison

Research date: **2026-08-20**. Prices are provider list prices in USD before tax and currency conversion. Revalidate before production cutover.

## Result

| Candidate | Mandatory gates | Weighted score | Verdict |
|---|---|---:|---|
| **Render Starter, Frankfurt** | Pass | **95/100** | Selected for proof |
| Google Cloud Run, Belgium | **Fail: cost ceiling** with safe background CPU | 85/100 | Rejected for this workload |
| Railway Hobby, EU | **Fail: no continuous health endpoint monitoring** | 79/100 | Rejected |
| DigitalOcean App Platform | Not evaluated | — | Not needed; product preference says fallback only |

Mandatory failures override weighted score.

## Scoring

| Category (weight) | Render | Cloud Run | Railway |
|---|---:|---:|---:|
| Infrastructure automation and recovery (25) | 5 → 25 | 5 → 25 | 3 → 15 |
| Deployment safety and rollback (15) | 5 → 15 | 4 → 12 | 5 → 15 |
| Straumheim runtime fit (20) | 5 → 20 | 5 → 20 | 5 → 20 |
| Observability and notifications (15) | 4 → 12 | 5 → 15 | 3 → 9 |
| Cost predictability (15) | 5 → 15 | 1 → 3 | 4 → 12 |
| EU, security, portability (10) | 4 → 8 | 5 → 10 | 4 → 8 |
| **Total** | **95** | **85** | **79** |

## Render

### Evidence

- Render sends health checks every few seconds to every running web-service instance. After 15 seconds of consecutive failure it removes the instance from traffic; after 60 seconds it restarts it and can notify the operator. New deployments receive no traffic until all new instances pass; after 15 minutes an unhealthy deploy is canceled and the previous instances continue serving ([Health Checks](https://render.com/docs/health-checks)).
- Git-backed services can deploy after CI checks pass. Render documents zero-downtime deployment for services without persistent disks, keeps the old instance serving while the candidate starts, sends SIGTERM to the old process, and allows a 1–300 second shutdown delay ([Deploying on Render](https://render.com/docs/deploys)).
- Recent successful artifacts can be rolled back. For image-backed services, a digest is required for predictable rollback; mutable tags are pulled again ([Rollbacks](https://render.com/docs/rollbacks)).
- Frankfurt is an available service region. Region cannot be changed in place ([Regions](https://render.com/docs/regions)).
- Starter service compute is **$7/month** for 512 MB and 0.5 CPU, prorated by second ([Pricing](https://render.com/pricing#services)).
- Runtime files are ephemeral by default, which is acceptable for stdout/external sinks but not the file sink ([Deploying on Render](https://render.com/docs/deploys#ephemeral-filesystem)).
- Secret files appear at `/etc/secrets/<filename>` and environment secrets are encrypted platform configuration ([Environment Variables and Secrets](https://render.com/docs/configure-environment-variables)).
- Email and Slack can notify deploy failure, image-pull failure, a running service becoming unhealthy, and recovery ([Notifications](https://render.com/docs/notifications)).
- Blueprint fields include `region`, `plan`, `autoDeployTrigger: checksPass`, `healthCheckPath`, and `maxShutdownDelaySeconds` ([Blueprint YAML Reference](https://render.com/docs/blueprint-spec)).

### Fit and caveats

- **Passes all gates at $7/month** before bandwidth, tax, destination DB, and domain.
- A Git-backed Docker service is preferable to `:latest`: each deploy is tied to a commit/build artifact, automatic deployment can wait for CI, and rollback can reuse the artifact. A registry-backed alternative must use a digest and an explicit deploy hook because Render does not watch tag changes.
- The one-time secret-file upload is acceptable setup, not recurring maintenance. Secret values must never enter `render.yaml`.
- `/health` only proves process liveness today. Render can recover a wedged process, but cannot detect a failed sink while that endpoint remains 200.
- A Starter instance is single-instance. Health recovery can include a short interruption while replacement starts; zero event loss is not promised.

## Google Cloud Run

### Evidence

- Request-based billing allocates CPU only while requests run. Instance-based billing allocates CPU for the full lifecycle and is explicitly recommended for Go goroutines/background work. Idle instances may still terminate; combining instance-based billing with one minimum instance keeps one active with CPU ([Billing settings](https://cloud.google.com/run/docs/configuring/billing-settings)).
- Cloud Run supports startup, liveness, and readiness probes. Liveness failure SIGKILLs the instance and autoscaling creates a replacement; readiness failure removes traffic ([Health checks](https://cloud.google.com/run/docs/configuring/healthchecks)).
- The filesystem is in-memory and non-persistent. Instances receive SIGTERM with only 10 seconds before SIGKILL. Minimum instances can be shut down at any time ([Container runtime contract](https://cloud.google.com/run/docs/container-contract)).
- Revisions are immutable and support traffic splitting, tags, and rollback. Non-serving revisions cost nothing unless they retain minimum instances ([Manage revisions](https://cloud.google.com/run/docs/managing/revisions)).
- Belgium, Netherlands, Paris, Finland, Stockholm, and other EU regions are available ([Cloud Run pricing regions](https://cloud.google.com/run/pricing#tier-1)). Cloud Logging, Monitoring, uptime checks, alerts, Secret Manager, and managed HTTPS are native Google Cloud services.

### Safe cost calculation

Straumheim requires instance-based billing and one minimum instance so its timer-driven flush runs while no request is active. Using the documented Tier 1 rates for **1 vCPU + 0.5 GiB** for 730 hours:

```text
seconds/month                    = 730 × 3,600 = 2,628,000
CPU gross                        = 2,628,000 × $0.000018 = $47.304
Memory gross                     = 2,628,000 × 0.5 × $0.000002 = $2.628
Free CPU discount                = 240,000 × $0.000018 = $4.320
Free memory discount             = 450,000 × $0.000002 = $0.900
Estimated safe compute total     = $44.712/month
```

This excludes egress, Cloud Logging beyond allowances, Artifact Registry, Monitoring, destination DB, domain, tax, and currency conversion. Actual free-tier treatment is billing-account-wide.

### Verdict

Cloud Run is operationally strongest but **fails the EUR 20/month ceiling by more than 2×** in the configuration that safely runs Straumheim’s background goroutine. Request-based billing could be inexpensive, but CPU is unavailable outside requests and therefore does not satisfy runtime safety. A future synchronous/durable architecture could justify reevaluation.

## Railway

### Evidence

- Railway waits for HTTP 200 before activating a deployment, but its documentation states twice that the endpoint is **not monitored after the deployment goes live** ([Healthchecks](https://docs.railway.com/deployments/healthchecks)).
- Restart policy handles process exit. Paid plans can set unlimited `Always` restarts, but this does not detect a live deadlock or unhealthy HTTP path ([Restart Policy](https://docs.railway.com/deployments/restart-policy)).
- Rollback restores the previous image and variables. Hobby retains removed images for 72 hours ([Deployment Actions](https://docs.railway.com/deployments/deployment-actions), [Pricing Plans](https://docs.railway.com/pricing/plans#image-retention-policy)).
- Native project webhooks cover deployment failure/crash. CPU/RAM threshold monitors require Pro ($20/month) and still do not continuously probe `/health` ([Alerts guide](https://docs.railway.com/guides/alerts-crashes-failed-deploys)).
- Hobby is $5/month including $5 usage; resource rates are $10/GB-month RAM and $20/vCPU-month CPU ([Pricing Plans](https://docs.railway.com/pricing/plans)).

### Cost range

A continuously running service using 0.5 GB RAM costs about $5/month in memory. At average 0.05 vCPU, CPU adds about $1; at average 0.25 vCPU, it adds about $5. Because Hobby includes the first $5 usage, expected total is roughly **$6–10/month**, excluding egress, destination DB, domain, tax, and currency conversion.

### Verdict

Railway is inexpensive and pleasant for deployment, but it **fails the mandatory continuous-health gate**. Its own suggested workaround is deploying Uptime Kuma, which introduces another service to operate and still does not give native traffic removal based on the check. Crash restart is not equivalent to application health recovery.

## Why DigitalOcean was not evaluated

Render satisfies every mandatory requirement and the budget. M008 explicitly makes DigitalOcean App Platform a fallback only, so expanding the comparison would add work without affecting the decision.

## Sensitivity

Reconsider the winner if any of these change:

- Render removes affordable continuous health/restart or EU hosting;
- the workload requires multi-instance availability rather than automatic single-instance recovery;
- Cloud Run offers a safe sub-$20 always-on configuration for background work;
- Straumheim moves flushing into durable request-independent work and no longer needs an always-running application process; or
- production egress/static-IP requirements materially change total cost.
