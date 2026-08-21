# M012 GCP-native live proof

- **Status:** blocked before GCP provisioning
- **Date:** 2026-08-21
- **Target:** EU Cloud Run collector → Pub/Sub retry/DLQ → private Cloud Run writer → EU BigQuery

## Access result

Implementation and credential-free validation are complete, but the configured owner account `timo@partnerwithpropel.com` cannot refresh its token non-interactively:

```text
Reauthentication failed. cannot prompt during non-interactive execution.
Please run: gcloud auth login
```

Consequently no M012 Cloud Run, Pub/Sub, Artifact Registry, Secret Manager, Monitoring, IAM, BigQuery, budget, WIF, or state-bucket proof resource was created. Render remains fully de-provisioned (`render services` and `render projects` both return null).

## Validated before provisioning

- Collector and writer request contracts have fake-based unit/integration tests.
- Webhook, pixel, and Snowplow requests serialize complete Records and preserve IDs through the push writer.
- Publish and sink failures produce non-success HTTP outcomes.
- Go format, full unit tests, race tests, build, and vet pass.
- OpenTofu format/init/validate pass with Google provider 6.50.0.
- Workflow YAML parses and uses WIF rather than a key.
- Scratch container builds with a 1.335 MB build context after excluding OpenTofu artifacts.
- No service-account JSON, private key, state, tfvars, or plan is present in Git.

These checks do not replace live provider evidence.

## Provisioning

Follow `infra/gcp/README.md` to bootstrap the state bucket, Artifact Registry repository, deployment WIF identity, GitHub Environment variables, and proof plan. Use:

- a dedicated project or isolated proof prefixes;
- `environment=proof`;
- `region=europe-west1` or another explicitly approved EU region;
- `dataset_id=straumheim_m012_proof`;
- `delete_proof_data_on_destroy=true`;
- exact proof CORS origins;
- an immutable image digest;
- tested notification channels.

Capture project/resource names, image digest, revisions, plan summary, UTC timestamps, IDs, metric values, and cleanup results only. Never capture tokens, state contents, secret values, message payloads, or screenshots containing credentials.

## Live checks

### 1. IAM and topology

- Confirm collector has a public invoker binding; writer does not.
- Confirm unauthenticated writer POST returns 401/403 before application handling.
- Confirm push identity alone has writer invoker.
- Confirm collector identity only has topic publisher plus config-secret access.
- Confirm writer identity only has dataset writer plus config-secret access.
- Confirm no user-managed key exists for any runtime identity.
- Confirm two warm collector instances and independent writer scaling settings.

### 2. Protocol and identity

Send uniquely tagged:

1. webhook POST;
2. pixel GET;
3. Snowplow GET `/sp/i`;
4. Snowplow batch POST `/sp/tp2`.

For each, record HTTP outcome and returned/observed Record ID. Query BigQuery core fields, `payload`, and `flattened`; require exact ID and JSON matches. Check Pub/Sub message attributes expose the same `record_id` without logging payload.

### 3. Durable writer outage

In proof only, deploy a writer revision with invalid destination configuration or temporarily remove its dataset binding. Continue sending collector canaries.

Pass when:

- collector still confirms Pub/Sub acceptance;
- writer returns non-success and does not acknowledge messages;
- backlog and oldest age rise;
- writer/backlog alerts route to the owner;
- restoring the known-good writer drains all IDs without manual replay; and
- duplicates, if any, retain one Record ID and the documented SQL returns one analytical row.

### 4. Dead letter

Publish one deliberately malformed message directly to the proof topic. Do not use production traffic. Pass when bounded non-success attempts move it to the DLQ, the DLQ alert fires, and inspection records message metadata/error without payload exposure. Explicitly dispose of the message after evidence.

### 5. Replacement and rollback

Poll `/health` and submit canaries while replacing the collector revision. Record non-200 count and maximum latency. Deploy a failing candidate or route to a known bad proof revision, then restore the known-good digest using Cloud Run revision traffic. Pass only with explained behavior and no missing acknowledged IDs.

### 6. Monitoring and cost

Confirm real time series/policies for collector and writer 5xx, oldest unacked age, undelivered backlog, DLQ backlog, and budget thresholds. Record normal and outage/drain instance counts and usage inputs for the formula in `gcp-runbook.md`.

## Teardown

Review a destroy plan against the proof state prefix, then remove proof resources. Verify independently:

- no proof Cloud Run services/revisions;
- no proof topics/subscriptions or DLQ messages;
- no proof BigQuery dataset/table;
- no proof secrets/versions;
- no proof runtime identities/bindings/keys;
- no proof alert policies/budgets/images;
- no temporary credentials/configs; and
- no production-owned resource in the destroy result.

## Evidence table

| Criterion | Status | Evidence |
|---|---|---|
| Confirmed publish before input success | Pass locally | Fakes and all-protocol integration tests |
| Publish failure returns non-success | Pass locally | Unit/integration tests |
| Private authenticated writer | Blocked | Requires Cloud Run IAM proof |
| BigQuery confirmation before push acknowledgement | Pass locally | Push/sink tests; live provider result blocked |
| Writer outage queues and redelivers | Blocked | Requires live Pub/Sub/Cloud Run |
| Bounded retry and DLQ | Pass statically / blocked live | OpenTofu validates; provider behavior unobserved |
| Duplicate ID semantics | Pass contractually / blocked live | M011 evidence and M012 SQL; Pub/Sub duplicate unobserved |
| No post-request correctness work | Pass | Code review and synchronous-result tests |
| Keyless least-privilege IAM | Pass statically / blocked live | OpenTofu graph; applied policy unobserved |
| Repeatable EU immutable infrastructure | Pass statically / blocked live | OpenTofu/workflow validation; apply blocked |
| Warm collector replacement | Blocked | Requires live Cloud Run |
| Monitoring routes actionable alerts | Blocked | Policies validate; time series/notification unobserved |
| All protocols reach BigQuery | Pass locally / blocked live | Fake request path passes; GCP path unobserved |
| Rollback and teardown | Blocked | No proof resources created |
| Full local quality gates | Pass | Go/race/build/vet, tofu, workflow YAML, Docker |
