# GCP production deployment record

- **Status:** production custom domain live; automated seven-day soak in progress
- **Date:** 2026-08-22
- **Project:** `propel-data-hub`
- **Region/location:** `europe-west1` / BigQuery `EU`

## Owner-approved values

| Setting | Value |
|---|---|
| Collector domain | `collect.partnerwithpropel.com` |
| CORS | `*`, explicitly approved for collection from arbitrary origins |
| Dataset | `straumheim_prod` |
| Alert owner | `timo@partnerwithpropel.com` |
| Budget request | USD 30/month; implemented as 200 DKK in the billing account's required native currency |
| Prior rollback endpoint | None; domain was NXDOMAIN before deployment |
| DNS/cutover | Approved and applied: DNS-only `collect` CNAME to `ghs.googlehosted.com` |

Because no prior DNS record or reachable collector existed, DNS rollback is removal of the new `collect` CNAME, returning to the recorded NXDOMAIN state. There is no former endpoint to restore.

## Deployed production resources

OpenTofu production state is stored under `gs://propel-data-hub-straumheim-tofu/straumheim/production`. The applied plan created keyless collector/writer identities, two Cloud Run services, Pub/Sub retry/DLQ resources, the production BigQuery dataset, config secrets, five alert policies, a budget, and an Artifact Registry repository.

Immutable image:

```text
europe-west1-docker.pkg.dev/propel-data-hub/straumheim-production/straumheim
@sha256:95fe066fa2e88f9e3099842ec7afaba4fb3f4a7871b661083def041ece21c7ca
```

Endpoints/resources:

- generated collector URL: `https://straumheim-production-collector-ftavcxw7fa-ew.a.run.app`
- collector scaling: min 2, max 10
- writer scaling: min 0, max 10
- events topic: `straumheim-production-events`
- writer subscription: `straumheim-production-writer`
- DLQ topic/subscription: production dead-letter resources
- BigQuery: `propel-data-hub.straumheim_prod.events`
- alert channel: `Straumheim production alerts`
- budget: 200 DKK with 50/90/100% thresholds

A final OpenTofu plan returned `No changes`.

## Generated-URL canaries

Before DNS, `/health` returned HTTP 200 in 131 ms. Webhook, pixel, Snowplow GET, and Snowplow POST all returned HTTP 200 and produced exactly four BigQuery rows:

| Protocol | Record ID | Result |
|---|---|---|
| webhook | `01a02a29-86e5-79cc-8756-fcca78b4004a` | matching response ID; nested/flattened count 1 |
| pixel | `01a02a29-87e6-7aff-9f52-9c9f94fd61e7` | matching proof payload |
| Snowplow GET | `01a02a29-887e-783b-b1ac-ffd3b097ba10` | matching proof payload |
| Snowplow POST | `01a02a29-8934-7a5d-85b4-e11a5f3dd993` | nested/flattened count 4 |

## Custom-domain cutover

Cloudflare now serves DNS-only CNAME `collect` → `ghs.googlehosted.com`. The first certificate attempt had backed off before DNS was added, so the OpenTofu domain mapping was safely replaced after DNS propagation. Cloud Run then reported all conditions true:

```text
Ready: True
CertificateProvisioned: True
DomainRoutable: True
```

HTTPS health returned HTTP 200 in 95 ms using TLS 1.3. Google Trust Services certificate details:

```text
CN/SAN: collect.partnerwithpropel.com
valid: 2026-08-24 through 2026-11-22
issuer: Google Trust Services WR3
```

At `2026-08-24T07:34:51Z`, custom-domain webhook, pixel, Snowplow GET, and Snowplow POST all returned HTTP 200. BigQuery returned exactly four matching rows:

```text
webhook       01a032b1-8b96-70fc-bb9d-7e7f2cb225bd
pixel         01a032b1-8c63-746b-80b3-95ce32216126
snowplow GET  01a032b1-8d35-7471-890c-ad723f8bb7e3
snowplow POST 01a032b1-8dec-7087-8d89-c233ce23f3d1
```

Nested and flattened values matched for webhook and Snowplow POST.

## Automated soak

The seven-day soak began at the first successful scheduled canary on `2026-08-24T07:43:32Z` and cannot complete before `2026-08-31T07:43:32Z`.

OpenTofu now manages:

- a five-minute Cloud Scheduler POST to the custom-domain webhook with `proof_id=m012-production-soak`;
- a 60-second SSL-validating uptime check from Europe, USA, and Asia-Pacific;
- an alert after two minutes of failed HTTPS checks.

A manual scheduler run succeeded and its first row appeared in `straumheim_prod.events` with one unique generated Record ID. Uptime time series reported `True` from Belgium, Oregon, Iowa, and Singapore. The final production OpenTofu plan returned `No changes`.

Do not remove historical Render files or retire any former host until the soak completes, even though Render currently has no live resources.
