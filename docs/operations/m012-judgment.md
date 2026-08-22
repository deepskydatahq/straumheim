# M012 product judgment

Judgment date: 2026-08-22

Result: **blocked on production cutover — all 15 implementation/proof criteria pass**

| # | Mission criterion | Result | Evidence |
|---:|---|---|---|
| 1 | HTTP success follows confirmed Pub/Sub message with same ID | Pass | Publisher waits for all results; four public protocols and BigQuery IDs match |
| 2 | Publish failure never falsely acknowledges | Pass | Unit/integration failure tests return HTTP 500; collector acknowledgement boundary proven |
| 3 | Private writer rejects unauthenticated callers | Pass | Unauthenticated writer POST returned 403; push identity was sole invoker |
| 4 | Writer acknowledges only after BigQuery result | Pass | Exact response IDs/JSON appeared after push 204 behavior; sink tests cover failure |
| 5 | Writer outage queues and recovers | Pass | Five accepted IDs were absent during IAM outage and all delivered automatically after restore |
| 6 | Bounded retry and terminal DLQ | Pass | Malformed message logged attempts 1–5 and reached DLQ with safe metadata |
| 7 | Duplicate delivery preserves ID and deduplicates | Pass | Same ID produced two raw rows and one `ROW_NUMBER` analytical row |
| 8 | No correctness-critical post-request work | Pass | Synchronous publish/append contract ran under request-based Cloud Run |
| 9 | Runtime IAM is keyless and least privilege | Pass | Applied topic/dataset/writer bindings and zero user-managed runtime keys |
| 10 | Infrastructure is repeatable, EU, secret-safe, immutable | Pass | Digest image, GCS state, environment isolation, dynamic billing currency, clean final plan |
| 11 | Warm collector replacement and writer scaling | Pass | Two idle collectors, 140/140 health responses, 14/14 BigQuery canaries during replacement |
| 12 | Monitoring receives actionable signals | Pass with note | Live instance/DLQ time series, five enabled policies, email channel, DKK budget; email receipt unconfirmed |
| 13 | EU all-protocol Cloud Run-to-BigQuery proof | Pass | Webhook, pixel, Snowplow GET/POST IDs and JSON matched |
| 14 | Rollback and teardown protect production data | Pass | Revision rollback canary matched; 43-resource destroy and independent empty inventories |
| 15 | Local quality and container/infrastructure gates | Pass | gofmt, test, race, build, vet, Docker, tofu, workflow YAML, TOML and secret scans |

## Completed artifacts and evidence

- Request-scoped architecture decision
- Pub/Sub v2 confirmed publisher and push writer with fakes/tests
- Default, collector, and writer runtime modes
- All-protocol credential-free and live integration tests
- Prometheus publish/push and last-delivery metrics
- Keyless OpenTofu Cloud Run/Pub/Sub/BigQuery/IAM/Secret Manager/Monitoring/budget stack
- WIF Artifact Registry digest deployment workflow
- Bootstrap, rollback, teardown, alert, cost, incident, DNS, and soak runbooks
- Complete live proof at `docs/operations/gcp-proof.md`
- Render service/project inventories remain empty

## Why the mission remains blocked

The technical profile is production-ready and the disposable proof is removed, but the mission outcome says Straumheim **runs** on the production GCP path. No production DNS change was authorized in this execution window, and required owner-specific values are intentionally not inferred:

- production collector domain and exact CORS origins;
- DNS owner and rollback endpoint;
- production dataset ID/retention and data owner;
- alert notification owners/channels;
- approved monthly budget in billing-account currency;
- cutover window and seven-day soak start;
- former host retirement authority.

## Unblock condition

1. Owner supplies/approves the production values above and explicitly authorizes the DNS window.
2. Apply the production state with `delete_proof_data_on_destroy=false` and protected GitHub Environment approval.
3. Execute custom-domain canaries, counters, alerts, rollback gates, and seven-day soak in `gcp-runbook.md`.
4. Retire the former host and historical Render files only after soak approval.
5. Complete M012-E005-S002, E005, and M012, then repeat this judgment.
