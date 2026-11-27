# M012 GCP-native live proof

- **Status:** complete — EU proof and teardown passed
- **Date:** 2026-08-22
- **Project:** `propel-data-hub`
- **Region/location:** Cloud Run and Artifact Registry `europe-west1`; BigQuery `EU`
- **Path:** public Cloud Run collector → Pub/Sub retry/DLQ → private Cloud Run writer → BigQuery

## Provisioning evidence

OpenTofu used the GCS state prefix `gs://propel-data-hub-straumheim-tofu/straumheim/proof`, Google provider 6.50.0, and image:

```text
europe-west1-docker.pkg.dev/propel-data-hub/straumheim-proof/straumheim
@sha256:95fe066fa2e88f9e3099842ec7afaba4fb3f4a7871b661083def041ece21c7ca
```

Initial full plan: 29 adds, one metadata update, zero destroys. Final reconciled plan returned `No changes`.

Created topology:

| Resource | Evidence |
|---|---|
| Collector | `straumheim-proof-collector`, min 2/max 4, public invoker check disabled |
| Writer | `straumheim-proof-writer`, min 0/max 4, no public binding |
| Topic/subscription | `straumheim-proof-events` / `straumheim-proof-writer` |
| DLQ | `straumheim-proof-dead-letter` plus inspection subscription |
| BigQuery | `straumheim_m012_proof.events` |
| Runtime identities | separate collector, writer, and push accounts; zero user-managed keys |
| Monitoring | collector/writer 5xx, backlog, oldest age, and DLQ policies wired to a disposable email channel |
| Budget | 25 DKK project budget at 50/90/100%; currency read from the billing account |

Two provider constraints were found and fixed during proof:

1. Organization domain-restricted sharing rejected an `allUsers` binding. The collector now uses Cloud Run's documented `invoker_iam_disabled` public-service mechanism; writer IAM remains enforced.
2. Billing APIs required user-project quota attribution and the billing account's actual DKK currency. The provider now sets `billing_project`/`user_project_override`, enables Cloud Billing, and derives currency from the billing account.

The externally bootstrapped GCS state bucket is retained as an approved production prerequisite; its proof prefix was removed during teardown. Required APIs remain enabled because OpenTofu sets `disable_on_destroy=false` for shared project capabilities.

## IAM evidence

- Unauthenticated writer POST returned HTTP 403 before application code.
- Writer Cloud Run IAM contained only `straumheim-proof-push@...` as `roles/run.invoker`.
- Events topic IAM contained only the collector runtime identity as publisher.
- Writer had dataset-scoped `roles/bigquery.dataEditor` plus access to its own config secret.
- Collector had topic publisher plus access to its own config secret.
- Pub/Sub service agent had only push token-minting and DLQ forwarding permissions required by those features.
- `gcloud iam service-accounts keys list --managed-by=user` returned `[]` for collector, writer, and push identities.
- No service-account JSON was created or mounted.

## Protocol and BigQuery proof

Collector URL health returned HTTP 200 in 114 ms. Writer remained private.

At `2026-08-22T09:27:13Z`, all enabled protocols returned HTTP 200:

| Protocol | Proof tag | BigQuery Record ID | JSON result |
|---|---|---|---|
| webhook | `m012-webhook-20260822T092713Z` | `01a028cb-b29d-7195-ba39-bbe7daf431eb` | nested and flattened count `1` |
| pixel | `m012-pixel-20260822T092713Z` | `01a028cb-b35f-7ee3-b2c4-dc064232f513` | payload matched |
| Snowplow GET | `m012-snowplow-get-20260822T092713Z` | `01a028cb-b3fe-78dc-9a4b-2e803d122176` | payload matched |
| Snowplow POST | `m012-snowplow-post-20260822T092713Z` | `01a028cb-b4ae-7ed5-a8ee-22f63df14d43` | nested and flattened count `4` |

A parameterized BigQuery query returned exactly four rows and the webhook response ID matched its stored row.

## Durable outage and recovery

The push identity's writer invoker binding was removed after IAM propagation. Five webhook requests were still durably accepted by the collector with IDs:

```text
01a028d1-4312-7b16-85aa-099b59eb8405
01a028d1-43af-74a2-b068-1434922f2059
01a028d1-4436-718c-a2ef-4f13c1cce344
01a028d1-44b2-7248-b651-65fa26dd8f90
01a028d1-4533-7e3c-907f-f08f371b2bcd
```

A query during the outage returned zero rows. The binding was restored through OpenTofu before the DLQ attempt bound; 15 seconds later BigQuery returned all five exact IDs without manual replay. Writer request logs recorded both rejected edge requests and successful 204 push acknowledgements.

A longer preliminary IAM outage intentionally crossed the five-attempt bound and moved five valid proof messages to the DLQ. They were inspected and acknowledged before the dedicated malformed-message drill. This also demonstrated that IAM outage duration must stay below the configured dead-letter bound if automatic delivery rather than operator replay is desired.

## Bounded failure and DLQ

A deliberately malformed message was published as message `20630539836204761` with proof attribute `m012-malformed-20260822`.

Writer logs recorded five bounded attempts at 10–23 second intervals:

```text
delivery_attempt=1 ... unmarshal record
...
delivery_attempt=5 ... unmarshal record
```

No payload was logged. Pub/Sub forwarded the message to the DLQ, where the inspection subscription exposed the retained proof attribute. Monitoring reported DLQ backlog value `1` for consecutive minute samples and the enabled DLQ policy referenced the proof email channel. The alerts API had not exposed an incident before teardown, so provider time-series/policy wiring is proven but email receipt was not independently confirmed in this session.

## Duplicate bound

The same canonical Record ID `m012-controlled-duplicate-20260822` was published twice. BigQuery returned:

```text
raw_rows=2
deduplicated_rows=1
```

using `ROW_NUMBER() OVER (PARTITION BY id ORDER BY received_at DESC)`. M012 remains explicitly at-least-once.

## Replacement and rollback

Cloud Run metrics showed two idle collector instances and an independently scaling writer.

During a collector revision replacement:

- 140 health requests at 500 ms intervals all returned HTTP 200;
- maximum health latency was 152 ms;
- 14 interleaved webhook canaries all returned HTTP 200;
- all 14 response IDs appeared in BigQuery;
- revision `straumheim-proof-collector-00002-vcp` replaced `...-00001-4wn`.

Traffic was then rolled back 100% to `...-00001-4wn`. Health remained HTTP 200 and rollback canary ID `01a028d8-4eb2-732b-98fe-1394a6e4479d` appeared in BigQuery. A final OpenTofu apply removed the drill-only configuration and a subsequent plan was clean.

## Cost and monitoring evidence

- A 25 DKK budget was created with 50/90/100% thresholds and removed during teardown.
- Monitoring time series exposed collector/writer instance counts, Pub/Sub backlog/age, and DLQ backlog.
- Five alert policies were enabled and referenced the disposable email channel.
- Application logs exposed message ID, Record ID when decodable, delivery attempt, status, and error without payloads or credentials.
- The proof used two warm collector instances only for the short evidence window; no fixed monthly estimate is inferred from this run.

## Teardown

A reviewed destroy plan contained 43 managed proof resources and zero changes/adds. OpenTofu destroy completed successfully, then the disposable notification channel and all versioned proof-state objects were removed manually.

Independent verification returned:

- Cloud Run proof services: `[]`; former URL HTTP 404;
- Pub/Sub proof topics/subscriptions: `[]`;
- BigQuery proof dataset: HTTP 404;
- proof service accounts and secrets: `[]`;
- Artifact Registry proof repositories: `[]`;
- proof alert policies/channels and budget: `[]`;
- proof state prefix: no objects.

Retained intentionally for the production phase: enabled GCP APIs, owner ADC/WIF bootstrap work, and the empty versioned GCS state bucket. No proof event data, runtime identity, image, secret, IAM binding, alert, budget, or service remains.

## Mission evidence table

| Criterion | Result | Evidence |
|---|---|---|
| Confirmed publish before success and fail-closed publishing | Pass | unit/all-protocol tests plus public proof |
| Private authenticated writer and confirmed BigQuery append | Pass | unauthenticated 403, push-only IAM, exact BigQuery IDs |
| Outage queues and automatically recovers | Pass | zero rows during outage, five exact rows after IAM restore |
| Bounded retry and DLQ | Pass | five logged attempts and malformed DLQ message |
| Duplicate semantics | Pass | two raw rows, one deduplicated row |
| No post-request correctness dependency | Pass | implementation contract and Cloud Run request proof |
| Keyless least-privilege IAM | Pass | applied policies and empty user-key inventories |
| Repeatable EU immutable infrastructure | Pass | digest deployment, remote state, clean reconciled plan |
| Warm replacement and independent scaling | Pass | 140/140 health, 14/14 canaries, instance metrics |
| Monitoring and budget signals | Pass with noted notification limitation | live time series/policies/channel and DKK budget; email receipt unconfirmed |
| All protocols reach BigQuery | Pass | four protocol rows with matching IDs/JSON |
| Rollback and teardown | Pass | revision rollback canary and complete independent cleanup |
| Full quality gates | Pass | Go/race/build/vet, OpenTofu, workflow YAML, Docker |
