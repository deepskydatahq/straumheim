# GCP production deployment record

- **Status:** deployed and validated on generated URL; custom-domain DNS/certificate pending
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
| DNS/cutover | Explicitly approved; external Cloudflare change still requires owner action |

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

## Required DNS action

Cloud Run created the domain mapping, but certificate issuance cannot start until Cloudflare contains:

```text
Type:   CNAME
Name:   collect
Target: ghs.googlehosted.com
Proxy:  DNS only (not proxied) during certificate provisioning
TTL:    Auto or 300 seconds
```

Current mapping status:

```text
Ready: Unknown / CertificatePending
CertificateProvisioned: Unknown / CertificatePending
DomainRoutable: True
```

After the DNS record resolves, wait for the managed certificate, then run all four canaries through `https://collect.partnerwithpropel.com`, verify matching production BigQuery rows, and begin the seven-day soak. Do not remove historical Render files or retire any former host until the soak completes, even though Render currently has no live resources.
