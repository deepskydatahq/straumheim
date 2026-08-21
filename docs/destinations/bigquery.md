# BigQuery destination

Status: implementation contract for M011. Last reviewed 2026-08-21.

## Decision

Use **direct BigQuery Storage Write API delivery** as Straumheim's primary analytical destination. Use the API's default stream with at-least-once semantics.

This is preferable to staging JSONL or Parquet in Cloud Storage for the current small tracking setup because:

- default-stream rows are committed and queryable with low latency;
- there is no object rotation, small-file compaction, load-job scheduler, or external-table lifecycle;
- one pipeline batch maps to one append request;
- BigQuery stores/query-optimizes the rows after ingestion; and
- two Render instances can append to the shared default stream without coordinating application stream offsets.

Google recommends the Storage Write API for new streaming projects and documents the default stream as immediately queryable, at-least-once ingestion. Exactly-once behavior requires application-created committed streams and offsets, which is deliberately out of scope.

Official references:

- [Storage Write API introduction](https://cloud.google.com/bigquery/docs/write-api)
- [Stream with the Storage Write API](https://cloud.google.com/bigquery/docs/write-api-streaming)
- [Storage Write best practices](https://cloud.google.com/bigquery/docs/write-api-best-practices)
- [BigQuery JSON type](https://cloud.google.com/bigquery/docs/json-data)
- [BigQuery pricing](https://cloud.google.com/bigquery/pricing)

## When Cloud Storage makes more sense

Add a separate Cloud Storage sink when the primary requirement becomes immutable low-cost archive, replay, or Parquet interchange. Cloud Storage is not selected as the primary path because newline-delimited JSON requires object boundaries and naming, delayed load jobs or external tables, explicit schema handling, and small-file management. See [loading JSON from Cloud Storage](https://cloud.google.com/bigquery/docs/loading-data-cloud-storage-json).

A future archive sink can fan out alongside BigQuery; it should not be hidden inside this sink.

## Delivery contract

- **Stream:** table default stream.
- **Visibility:** rows are committed after a successful append result and are available to queries without a separate finalize/commit call.
- **Guarantee:** at least once. Ambiguous transport outcomes or upstream retries can produce duplicate IDs.
- **Batching:** each `Sink.Write` call serializes all supplied Records and issues one append.
- **Success:** `Write` returns only after `AppendResult.GetResult` succeeds.
- **Failure:** serialization, immediate append, and asynchronous append-result errors are returned to the pipeline with context.
- **Durability boundary:** current Straumheim acknowledges into an in-memory buffer. A process death before successful delivery can still lose accepted records; M009 owns durable acceptance and retry.
- **Deduplication key:** `id`. Analytical consumers that require one row per event should use `ROW_NUMBER() OVER (PARTITION BY id ORDER BY received_at DESC)` or a curated deduplicated table/view.

Do not add an outer automatic retry around an append whose result is unknown. The managed writer handles connection behavior; application-level durable retry must preserve Record IDs and account for possible duplicate appends.

## Dataset and table contract

The dataset must already exist. Straumheim does not infer or create datasets because dataset location is a data-residency and billing decision. On startup, the sink:

1. reads dataset metadata;
2. verifies its location equals configured `location` case-insensitively;
3. reads table metadata; and
4. creates the table only when it is absent.

The table is time-partitioned daily on `timestamp` and clustered by `protocol`, then `source`.

| Column | BigQuery type | Required | Source |
|---|---|---:|---|
| `id` | STRING | yes | `Record.ID` |
| `timestamp` | TIMESTAMP | yes | event/collector timestamp |
| `received_at` | TIMESTAMP | no | collector receipt timestamp |
| `device_time` | TIMESTAMP | no | optional device timestamp |
| `protocol` | STRING | no | input protocol |
| `source` | STRING | no | source/application |
| `schema` | STRING | no | schema name |
| `vendor` | STRING | no | schema vendor |
| `schema_version` | STRING | no | schema version |
| `is_valid` | BOOL | no | optional validation result |
| `ip` | STRING | no | collector metadata |
| `user_agent` | STRING | no | collector metadata |
| `referer` | STRING | no | collector metadata |
| `payload` | JSON | no | original event object |
| `flattened` | JSON | no | flattened event object |

Arbitrary tracking properties remain in native JSON instead of creating BigQuery columns at runtime. This gives stable ingestion while retaining JSON operators and dot-path querying.

## Protobuf mapping

The Storage Write API accepts protobuf wire rows. Straumheim uses a runtime proto2 descriptor matching the stable table fields:

- strings and bool map directly;
- TIMESTAMP values are signed microseconds since Unix epoch;
- nullable values are omitted when absent;
- `payload` and `flattened` are JSON-encoded strings sent to JSON destination fields.

Serialization is deterministic for testability. JSON encoding failures identify the Record ID.

## Configuration

```yaml
sinks:
  - name: warehouse
    type: bigquery
    mode: batch
    project: my-gcp-project
    dataset: analytics
    table: events
    location: EU
    max_inflight_requests: 1 # optional managed-writer bound
```

Authentication is intentionally not part of the YAML destination contract. Google client libraries discover [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials). On Render, mount a service-account JSON file as `/etc/secrets/gcp-service-account.json` and set:

```text
GOOGLE_APPLICATION_CREDENTIALS=/etc/secrets/gcp-service-account.json
```

Never commit, print, or include the JSON key in product evidence.

## IAM

Separate one-time provisioning from runtime permissions:

- an administrator enables BigQuery/Storage Write APIs, creates the EU dataset, creates the service account, and controls key lifecycle;
- the runtime identity needs dataset-scoped permissions to inspect dataset/table metadata, create the configured table if absent, update table data, and use Storage Write connections;
- prefer a custom dataset-level role containing only required permissions, or use the narrowest documented BigQuery data roles accepted by organizational policy;
- do not grant Owner/Editor, billing administration, or organization-wide permissions to Straumheim.

Review [BigQuery IAM roles and permissions](https://cloud.google.com/bigquery/docs/access-control) during live setup because role composition and organization policy can change.

## Cost boundary

BigQuery cost is separate from the $14/month Render compute decision. Record:

- Storage Write ingestion usage beyond any current free allowance;
- stored bytes;
- query bytes processed or reservation capacity; and
- network egress from Render to Google Cloud.

Set a project billing budget, partition every query by `timestamp`, and avoid `SELECT *` over unbounded dates. Revalidate current [BigQuery pricing](https://cloud.google.com/bigquery/pricing) before production cutover.

## Query examples

Recent raw events:

```sql
SELECT id, timestamp, protocol, source, payload
FROM `my-gcp-project.analytics.events`
WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY)
ORDER BY timestamp DESC;
```

Deduplicate IDs for analysis:

```sql
SELECT * EXCEPT(row_num)
FROM (
  SELECT *, ROW_NUMBER() OVER (
    PARTITION BY id ORDER BY received_at DESC
  ) AS row_num
  FROM `my-gcp-project.analytics.events`
  WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
)
WHERE row_num = 1;
```

JSON property:

```sql
SELECT
  id,
  JSON_VALUE(payload, '$.event') AS event_name,
  JSON_VALUE(payload, '$.user_id') AS user_id
FROM `my-gcp-project.analytics.events`
WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY);
```

## Disposable proof provisioning

Choose an explicitly approved project before running these commands. The examples use an EU multi-region dataset and a temporary service account:

```bash
export GCP_PROJECT="your-approved-project"
export BQ_DATASET="straumheim_m011_proof"
export BQ_TABLE="events"
export BQ_LOCATION="EU"
export BQ_SERVICE_ACCOUNT="straumheim-m011-proof"

# Human/admin setup.
gcloud auth login
gcloud config set project "$GCP_PROJECT"
gcloud services enable \
  bigquery.googleapis.com \
  bigquerystorage.googleapis.com

bq --location="$BQ_LOCATION" mk --dataset \
  "$GCP_PROJECT:$BQ_DATASET"

gcloud iam service-accounts create "$BQ_SERVICE_ACCOUNT" \
  --display-name="Disposable Straumheim M011 proof"

export BQ_SERVICE_ACCOUNT_EMAIL="${BQ_SERVICE_ACCOUNT}@${GCP_PROJECT}.iam.gserviceaccount.com"
bq add-iam-policy-binding "$GCP_PROJECT:$BQ_DATASET" \
  --member="serviceAccount:${BQ_SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/bigquery.dataEditor"

umask 077
gcloud iam service-accounts keys create \
  /tmp/straumheim-m011-proof-key.json \
  --iam-account="$BQ_SERVICE_ACCOUNT_EMAIL"
```

`roles/bigquery.dataEditor` is granted only on the disposable dataset, not the project. If organization policy requires a custom role, include the documented table metadata/create/update-data permissions and keep the binding dataset-scoped.

Before creating the Render service:

1. Upload `deploy/render/config.bigquery.example.yaml` as secret file `config.yaml`.
2. Upload `/tmp/straumheim-m011-proof-key.json` as secret file `gcp-service-account.json`.
3. Set `BIGQUERY_PROJECT`, `BIGQUERY_DATASET`, `BIGQUERY_TABLE`, and `BIGQUERY_LOCATION=EU`.
4. Set `GOOGLE_APPLICATION_CREDENTIALS=/etc/secrets/gcp-service-account.json`.
5. Verify that no key content appears in service events, build arguments, or logs.

The committed `render.yaml` declares only paths and destination placeholders. It never contains a credential value.

## Teardown

After evidence capture:

```bash
# Delete Render service/secret files first so the key is no longer in use.
bq rm --table --force "$GCP_PROJECT:$BQ_DATASET.$BQ_TABLE" || true
bq rm --dataset --force --recursive "$GCP_PROJECT:$BQ_DATASET"
gcloud iam service-accounts delete "$BQ_SERVICE_ACCOUNT_EMAIL" --quiet
rm -f /tmp/straumheim-m011-proof-key.json
```

Also confirm the Render workspace has no proof service and list active service-account keys if the account was intentionally retained. A key file is not considered removed until both the local copy and cloud key/service account are gone.
