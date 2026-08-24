# GCP-native production runbook

Status: implementation complete; live M012 proof and production cutover not yet approved.

Architecture: public Cloud Run collector → Pub/Sub → private Cloud Run writer → BigQuery. Infrastructure source: `infra/gcp/`.

## Owners and change record

Assign before proof or cutover:

| Role | Responsibility |
|---|---|
| Runtime owner | Cloud Run revisions, image digest, capacity, rollback |
| Delivery owner | Pub/Sub backlog/DLQ and BigQuery validation |
| IAM owner | WIF, service identities, bindings, emergency revocation |
| DNS owner | collector domain, TTL, cutover and rollback |
| Alert owner | notification channels, acknowledgement and incident command |
| Data owner | production dataset retention and deletion approval |

Record project, region, environment, image digest, OpenTofu state generation, collector/writer revisions, topic/subscription, dataset/table, notification channels, owners, UTC window, canary IDs, and rollback threshold. Never record tokens, key material, event payloads, or state contents.

## Pre-deployment gates

- [ ] M012 unit, race, build, container, workflow, and OpenTofu gates pass.
- [ ] Proof environment and production have separate names/state prefixes.
- [ ] GitHub Environment requires approval for production apply.
- [ ] WIF attribute condition allows only the intended repository/ref/environment.
- [ ] No user-managed service-account key exists for runtime or deployment.
- [ ] CORS origins are exact unless the data owner explicitly approves wildcard collection from arbitrary sites; record that exception in the change evidence.
- [ ] Pub/Sub retention exceeds the accepted maximum outage window.
- [ ] Retry min/max and DLQ attempts are approved.
- [ ] Monitoring channels receive a test notification.
- [ ] Budget and log retention are configured.
- [ ] Current collector endpoint, DNS, traffic baseline, and rollback target are recorded.

## Deploy and validate without production DNS

1. Run the GCP workflow with `apply=false`; review image digest and complete plan.
2. Approve and rerun with `apply=true`.
3. Confirm two warm collector instances (or approved equivalent), writer IAM privacy, and healthy revisions.
4. Verify an unauthenticated writer request is rejected by Cloud Run IAM.
5. Send unique webhook, pixel, Snowplow GET, and Snowplow POST canaries to the generated collector URL.
6. Confirm each collector response ID/message attribute and BigQuery Record ID/JSON.
7. Force writer failure in proof only. Confirm collector stays healthy, backlog/age rise, writer 5xx alerts fire, and messages drain automatically after recovery.
8. Send a permanently malformed proof message. Confirm bounded attempts and DLQ alert/subscription visibility.
9. Replace collector and writer revisions while polling health and canaries.
10. Roll back to the prior digest/revision and repeat one canary.
11. Soak proof traffic for at least 24 hours with external synthetic canaries.

Do not proceed when any acknowledged canary is absent, a queue drains only after manual replay, IAM allows public writer invocation, alerts do not route, or teardown cannot identify every proof resource.

## Alert semantics

| Signal | Initial threshold | Meaning | Action |
|---|---:|---|---|
| Collector Cloud Run 5xx | any sustained for 1 minute | durable publish not acknowledged | inspect collector/Pub/Sub; clients received failure |
| Writer Cloud Run 5xx | any sustained for 1 minute | message remains unacknowledged | inspect BigQuery/writer; do not restart collector |
| Oldest unacked age | 5 minutes for 5 minutes | delivery freshness objective breached | incident; compare retention and drain rate |
| Undelivered backlog | 1,000 for 5 minutes | outage or insufficient writer throughput | restore sink or increase safe writer capacity |
| DLQ backlog | any for 1 minute | terminal/malformed message | inspect metadata/error, fix, then explicitly replay or dispose |
| No confirmed delivery | workload-specific | stale path despite expected traffic | compare synthetic canary and Pub/Sub metrics |
| Budget | 50/90/100% | usage/configuration drift | inspect Cloud Run minimums, logs, egress, and BigQuery volume |

`/health` is process liveness. A BigQuery outage must not make the collector unhealthy or trigger an event-losing restart loop. Delivery health comes from Pub/Sub/provider metrics, writer HTTP outcomes, and last-delivery metrics/logs.

## Incident procedures

### Pub/Sub publish failure

Collector returns non-success, so there is no durable-acceptance claim. Check Pub/Sub API status, collector identity/topic IAM, quota, and project billing. Do not return synthetic success. Client retry can duplicate previously confirmed records in a multi-record request; deduplicate by ID.

### BigQuery/writer outage

Keep the collector serving. Verify backlog and oldest age rise. Restore writer revision, dataset IAM, table compatibility, or BigQuery service. Pub/Sub retries; confirm age/backlog return to zero and matching IDs arrive. Estimate drain time before increasing max instances or concurrency.

### Dead-letter message

Do not acknowledge the inspection subscription until disposition is recorded. Correlate message ID, Record ID when decodable, delivery attempts, and writer error logs. Never paste payload into tickets. Correct schema/config/code, replay with the same Record ID if appropriate, verify BigQuery, then acknowledge.

### Duplicate rows

At-least-once delivery is expected. Validate with:

```sql
SELECT * EXCEPT(row_number)
FROM (
  SELECT *, ROW_NUMBER() OVER (
    PARTITION BY id ORDER BY received_at DESC
  ) AS row_number
  FROM `project.dataset.events`
)
WHERE row_number = 1;
```

Do not claim exactly once.

## Cost model and budget

Use the official calculators/pricing pages immediately before approval; rates and free tiers change:

- Cloud Run: https://cloud.google.com/run/pricing
- Pub/Sub: https://cloud.google.com/pubsub/pricing
- BigQuery: https://cloud.google.com/bigquery/pricing
- Artifact Registry: https://cloud.google.com/artifact-registry/pricing
- Cloud Logging/Monitoring: https://cloud.google.com/stackdriver/pricing
- Calculator: https://cloud.google.com/products/calculator

Estimate monthly cost from measured values:

```text
collector = warm-instance idle allocation + request CPU/memory + requests
writer    = request CPU/memory during push + minimum instances (initially zero)
pubsub    = published + delivered + retained GiB and cross-region egress (keep regional path aligned)
bigquery  = Storage Write ingestion + retained storage + query bytes/slots
registry  = retained image GiB + network egress
operations = ingested logs/metrics above included allocations
```

Model normal, single-writer-outage, and 24-hour-backlog-drain scenarios. Set the OpenTofu budget below the maximum owner-approved surprise, then tune two warm collector instances only after measured latency/availability evidence. Exclude downstream analytics queries from collector runtime cost but budget them separately.

## Production DNS cutover

1. Lower TTL to 300 seconds at least one previous TTL window in advance.
2. Freeze unrelated collector/config changes.
3. Add and verify the production domain using the approved Cloud Run domain/load-balancer approach.
4. Keep the previous collector available and unchanged.
5. Send pre-cutover canaries and record Pub/Sub/BigQuery counters.
6. Update DNS, then validate TLS and every enabled protocol from multiple networks.
7. Compare collector success, Pub/Sub published/delivered, backlog/age/DLQ, writer 5xx, and BigQuery rows at 5, 15, 30, and 60 minutes.
8. Roll back DNS immediately for repeated collector failures, missing acknowledged IDs, invalid TLS/CORS/cookies, unexplained backlog growth, absent alerts, or material latency/error regression.

DNS propagation can split traffic. Use unique IDs; never infer loss from one host's request count alone.

## Soak and retirement

Keep the previous production collector available for at least seven days after cutover. Operate from alerts, not routine dashboards or SSH. At 24 hours and seven days review canary freshness, backlog/DLQ, errors, latency, instance counts, and cost.

For the managed production soak, enable the OpenTofu HTTPS uptime check and scheduled webhook canary. Confirm uptime points from Europe, USA, and Asia-Pacific remain true and query `proof_id = 'm012-production-soak'` for fresh, unique IDs. These automate evidence collection but do not waive the seven-day observation window.

Only after the soak and explicit owner approval:

1. verify no production DNS/traffic reaches the previous host;
2. revoke obsolete credentials and IAM;
3. remove obsolete Render deployment configuration (`render.yaml`, `deploy/render/`) in a dedicated reviewed story;
4. retire the former VM/service and unneeded volumes/snapshots;
5. raise DNS TTL;
6. record final revisions, digest, state, owners, cost baseline, and rollback outcome.

Render currently has no live services/projects; committed Render files are historical/fallback configuration until this gate passes.

## Proof teardown

Use an OpenTofu destroy plan for the proof state only. Verify removal of Cloud Run services/revisions, topics/subscriptions, proof dataset/table, config secret versions, runtime identities/bindings, alert policies, proof images, and state objects according to retention policy. Confirm production-owned datasets, topics, domains, channels, and state prefixes are absent from the destroy plan.
