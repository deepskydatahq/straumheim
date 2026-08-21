# Render-to-BigQuery proof

- **Status:** local GCP integration passed; Render deployment blocked by billing
- **Last access/proof check:** 2026-08-21
- **Target:** two Render Starter instances in Frankfurt → EU BigQuery dataset

## Current blockers

| Dependency | Result |
|---|---|
| Sink implementation and unit tests | Ready |
| Render CLI authentication | Available |
| Render Starter billing/payment | Blocked — prior workspace had no payment information; two paid instances cannot be created |
| GCP credentials | Available — user supplied a dataset-scoped service-account JSON; local mode was tightened to `0600` |
| Approved GCP proof project | Available — user explicitly approved `propel-data-hub` and `straumheim_test` |
| BigQuery CLI (`bq`) | Missing locally; proof used authenticated BigQuery REST metadata/table-data endpoints |
| EU proof dataset/table | Available — existing EU dataset `straumheim_test`; sink created `events` |

The Render create request returned HTTP 402 before creating a service. The BigQuery table and service-account key remain active for a resumed Render proof; they must be deleted after the final run.

## Observed local GCP integration

A credentialed build from commit `6f0dcfa` ran locally against the production BigQuery APIs at 2026-08-21 09:44–09:46 UTC:

- `BigQuerySink.Init` read dataset metadata, confirmed location `EU`, and created `propel-data-hub.straumheim_test.events`.
- Table metadata reported DAY partitioning on `timestamp` and clustering by `protocol, source`.
- Twelve HTTP webhook events were accepted and all twelve rows became visible through BigQuery `tabledata.list`.
- Returned IDs matched stored IDs, including first ID `01a02347-c39b-72e0-8e99-12d74187a037` and last ID `01a02347-c39d-7751-877f-c4eb3f0e9a5a`.
- For proof IDs `m011-local-20260821-00` through `-11`, nested `payload.nested.count` and `flattened.nested_count` both matched values 0 through 11.
- Starting with configured location `US` failed before serving traffic with: `dataset location "EU" does not match configured location "US"`.
- The runtime service account needed no query-job role; metadata and row verification used its dataset-scoped permissions.

This validates GCP authentication, table creation, stable metadata, batching through the running pipeline, JSON values, and actionable initialization errors. It does **not** substitute for the required public Render HTTPS/two-instance evidence.

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
| Two Render Starter instances healthy | Blocked | Render create returned HTTP 402; no service was created |
| HTTPS event ID matches BigQuery row | Blocked | Local HTTP ID/row match passed; public Render HTTPS remains |
| Payload and flattened JSON query correctly | Pass locally | Twelve nested/flattened values matched through BigQuery REST |
| Multi-record batch visible | Pass locally | Twelve rapidly submitted records appeared in the table |
| Duplicate semantics bounded | Blocked | Requires controlled live duplicate/query evidence |
| Failure is actionable and secret-safe | Pass locally | Wrong-location initialization failed clearly; secret scan was clean |
| All proof resources removed | Blocked | `events` table and proof service-account key remain for resumed proof |
