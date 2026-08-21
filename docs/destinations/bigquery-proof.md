# Render-to-BigQuery proof

- **Status:** two-instance Starter Render-to-BigQuery proof passed; owner-level GCP cleanup pending
- **Last access/proof check:** 2026-08-21
- **Target:** two Render Starter instances in Frankfurt → EU BigQuery dataset

## Current blockers

| Dependency | Result |
|---|---|
| Sink implementation and unit tests | Ready |
| Render CLI authentication | Available |
| Render Starter billing/payment | Available — two Frankfurt Starter instances were created, tested, and deleted |
| GCP credentials | Local/Render copies removed — cloud key revocation requires owner IAM permission |
| Approved GCP proof project | Available — user explicitly approved `propel-data-hub` and `straumheim_test` |
| BigQuery CLI (`bq`) | Missing locally; proof used authenticated BigQuery REST metadata/table-data endpoints |
| EU proof dataset/table | Table deleted — empty dataset deletion requires owner BigQuery permission |

The initial two-instance request returned HTTP 402, then succeeded after billing was added. Both the Free and Starter Render services were deleted. The BigQuery table and local key were removed; deletion of the now-empty dataset and cloud service-account key requires the owner permission that the runtime account intentionally lacks.

## Observed local GCP integration

A credentialed build from commit `6f0dcfa` ran locally against the production BigQuery APIs at 2026-08-21 07:44–07:46 UTC:

- `BigQuerySink.Init` read dataset metadata, confirmed location `EU`, and created `propel-data-hub.straumheim_test.events`.
- Table metadata reported DAY partitioning on `timestamp` and clustering by `protocol, source`.
- Twelve HTTP webhook events were accepted and all twelve rows became visible through BigQuery `tabledata.list`.
- Returned IDs matched stored IDs, including first ID `01a02347-c39b-72e0-8e99-12d74187a037` and last ID `01a02347-c39d-7751-877f-c4eb3f0e9a5a`.
- For proof IDs `m011-local-20260821-00` through `-11`, nested `payload.nested.count` and `flattened.nested_count` both matched values 0 through 11.
- Starting with configured location `US` failed before serving traffic with: `dataset location "EU" does not match configured location "US"`.
- The runtime service account needed no query-job role; metadata and row verification used its dataset-scoped permissions.

This validates GCP authentication, table creation, stable metadata, batching through the running pipeline, JSON values, and actionable initialization errors.

## Observed public Render Free proof

A one-instance Frankfurt Free service provided public-path evidence while preserving the two-instance billing blocker:

- Service `srv-da405g0jo6nc73dd6gng`, deployment `dep-da405ggjo6nc73dd6hi0`, commit `ef80572`, URL `https://straumheim-m011-proof.onrender.com`.
- Health returned HTTP 200; the first request after deployment took 8.140 seconds on Free.
- Twelve HTTPS webhook requests returned HTTP 200 and IDs from `01a0234d-2a0b-7ab5-8a97-1529747fa6da` through `01a0234d-2d95-7247-bf18-2cc53e0adf82`.
- BigQuery exposed all twelve matching IDs and proof tags `m011-render-free-20260821-00` through `-11`; nested payload and flattened values matched 0 through 11.
- A controlled direct-sink append wrote ID `m011-controlled-duplicate-20260821` twice. BigQuery exposed two raw rows, confirming the documented at-least-once boundary and need to deduplicate by ID.
- The first live deploy exposed that `ManagedStream.Close` returns `io.EOF` on a normal close. Commit `58900a9` normalized that documented behavior, added a regression test, and passed all Go gates. A Render restart then logged `flushing pipeline` followed by `shutdown complete` with no error.
- Render service and uploaded secret files were deleted. The former URL returns HTTP 404 and `render services` returns no services.

This passes the public HTTPS, BigQuery row/JSON, batch, duplicate-bound, and graceful-close checks.

## Observed two-instance Starter proof

After billing was enabled, the intended Frankfurt topology passed:

- Service `srv-da40el8ae00c739dcgfg`, deployment `dep-da40em0ae00c739dci40`, commit `9dec7cd`, plan Starter, `numInstances=2`.
- Render listed initial instances `...-fqkgz` and `...-th6rg`; both independently logged successful sink initialization and server startup.
- Twenty public HTTPS webhook requests returned HTTP 200 and IDs from `01a0235f-8f89-7d62-ab93-926545b9c5e6` through `01a0235f-9499-7aa4-b5b9-55ed745144c6`.
- BigQuery exposed all twenty matching IDs and proof tags `m011-render-starter-20260821-00` through `-19`; nested payload and flattened values matched 0 through 19.
- During a service restart, a 60-second monitor made 120 health requests at 500 ms intervals. All 120 returned HTTP 200; maximum latency was 121 ms.
- Render replaced the instances with `...-9fzhb` and `...-xv6qr`, and both replacements logged successful startup. This demonstrates healthy-peer availability during replacement.
- The Starter service and uploaded secret files were deleted. Render lists no services and the former URL returns HTTP 404.

## Cleanup result

- Render proof service/secrets: deleted.
- BigQuery `events` table: deleted; subsequent table listing was empty.
- Local service-account JSON: securely removed with `shred`; temporary gcloud credential/config files were removed.
- Empty dataset `propel-data-hub.straumheim_test`: deletion returned HTTP 403 because the runtime account correctly lacks `bigquery.datasets.delete`.
- Cloud key `fd00afac12905a5e67885c2aced5241f640b5db1`: deletion returned permission denied because the runtime account correctly lacks `iam.serviceAccountKeys.delete`.

An owner must delete the cloud key (or the whole proof service account) and the empty dataset before cleanup passes. No private-key copy remains in the worktree, local Downloads directory, temporary gcloud config, or Render workspace.

## Proof procedure

Follow the provisioning and teardown commands in [bigquery.md](bigquery.md), then:

### 1. Deploy

- Deploy the mission branch with the committed `render.yaml` using two Starter instances.
- Upload `config.bigquery.example.yaml` as Render secret file `config.yaml`.
- Upload the temporary key as secret file `gcp-service-account.json`.
- Record only service/deployment IDs, commit, region, dataset/table names, and UTC timestamps.
- Confirm both instances become healthy and no credentials appear in logs.

### 2. Synthetic event

```bash
export PROOF_URL="https://<render-service>.onrender.com"
export PROOF_ID="m011-single-$(date -u +%Y%m%dT%H%M%SZ)"

curl --fail-with-body --show-error \
  -H 'Content-Type: application/json' \
  -d "{\"event\":\"m011_bigquery_proof\",\"proof_id\":\"$PROOF_ID\",\"nested\":{\"count\":1}}" \
  "$PROOF_URL/webhook"
```

Capture the response Record ID, then query:

```sql
SELECT
  id,
  timestamp,
  received_at,
  protocol,
  JSON_VALUE(payload, '$.proof_id') AS proof_id,
  JSON_VALUE(payload, '$.nested.count') AS nested_count,
  JSON_VALUE(flattened, '$.nested_count') AS flattened_count
FROM `<project>.<dataset>.<table>`
WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)
  AND JSON_VALUE(payload, '$.proof_id') = '<proof-id>';
```

Pass only when ID and JSON values match the HTTP response/payload.

### 3. Batch behavior

Send at least 10 uniquely tagged webhook events quickly enough to enter one pipeline flush. Query all proof IDs and inspect `straumheim_records_delivered_total{sink="warehouse"}`. Unit tests prove one append call per `Sink.Write`; the live evidence proves all rows become queryable.

### 4. Duplicate bound

The default stream is at-least-once. Do not manufacture a transport failure against production. For the disposable proof, either:

- deliberately resend the same serialized Record ID through a test helper; or
- demonstrate the deduplication SQL against a controlled duplicate inserted with proof credentials.

Record whether two raw rows exist and show the `ROW_NUMBER() OVER (PARTITION BY id ...)` query returns one analytical row. Do not claim exactly once if no duplicate appears.

### 5. Failure evidence

With the disposable service only, remove dataset write permission or point to a nonexistent table revision and verify:

- sink initialization or append fails with an actionable `bigquery sink:` error;
- the service/deploy does not expose credentials;
- failed-delivery metrics/logs are visible; and
- restoring permission/config restores delivery.

The current pipeline does not retry failed batches. Record the loss risk and reference M009.

### 6. Cleanup

Delete in this order:

1. Render proof service and secret files.
2. BigQuery proof table/dataset.
3. Service account/key.
4. Local key file.
5. Temporary branches/worktrees.

Verify Render lists no proof service, BigQuery lists no proof dataset, IAM lists no proof account/key, and the former service URL no longer serves.

## Evidence table

Replace `blocked` only with observed facts:

| Criterion | Status | Evidence |
|---|---|---|
| Two Render Starter instances healthy | Pass | Two instances started; replacement produced 120/120 health responses with no outage |
| HTTPS event ID matches BigQuery row | Pass | Twelve public Render HTTPS IDs matched twelve BigQuery rows |
| Payload and flattened JSON query correctly | Pass | Twelve nested/flattened values matched through BigQuery REST |
| Multi-record batch visible | Pass | Twelve rapidly submitted HTTPS records appeared in the table |
| Duplicate semantics bounded | Pass | Controlled ID produced two raw rows; docs deduplicate by ID |
| Failure is actionable and secret-safe | Pass | Wrong-location initialization failed clearly; secret scan was clean |
| All proof resources removed | Blocked | Render/table/local key removed; owner must delete empty dataset and cloud IAM key/account |
