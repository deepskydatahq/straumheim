# Render-to-BigQuery proof

- **Status:** blocked before provisioning
- **Last access check:** 2026-08-21
- **Target:** two Render Starter instances in Frankfurt → EU BigQuery dataset

## Current blockers

| Dependency | Result |
|---|---|
| Sink implementation and unit tests | Ready |
| Render CLI authentication | Available |
| Render Starter billing/payment | Blocked — prior workspace had no payment information; two paid instances cannot be created |
| GCP user session | Blocked — configured account requires interactive reauthentication |
| Approved GCP proof project | Unconfirmed — local config names `propel-data-hub`, but M011 must not create resources there without explicit approval |
| BigQuery CLI (`bq`) | Missing locally; install the Google Cloud CLI BigQuery component or use API/console |
| EU proof dataset/service account | Not created |

No BigQuery dataset, table, service account, key, or Render proof service has been created for M011. No live result is claimed.

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
| Two Render Starter instances healthy | Blocked | Requires payment information |
| HTTPS event ID matches BigQuery row | Blocked | Requires approved GCP project and credentials |
| Payload and flattened JSON query correctly | Blocked | Requires live table |
| Multi-record batch visible | Blocked | Requires live pipeline |
| Duplicate semantics bounded | Blocked | Requires controlled live duplicate |
| Failure is actionable and secret-safe | Blocked | Requires disposable permission/config drill |
| All proof resources removed | Pass so far | None were created during access check |
