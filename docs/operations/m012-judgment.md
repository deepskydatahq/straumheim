# M012 product judgment

Judgment date: 2026-08-21

Result: **blocked — 6 mission criteria pass locally/statically; 9 require live GCP or production evidence**

| # | Mission criterion | Result | Evidence |
|---:|---|---|---|
| 1 | HTTP success follows confirmed Pub/Sub message with same ID | Pass | Publisher waits for all results; all-protocol request integration preserves IDs |
| 2 | Publish failure never falsely acknowledges | Pass | Publisher/input failure tests return wrapped error and HTTP 500 |
| 3 | Private writer rejects unauthenticated callers | **Blocked** | OpenTofu has only push invoker binding, but applied Cloud Run IAM is unobserved |
| 4 | Writer acknowledges only after BigQuery result | Pass | Push handler calls synchronous sink write and returns 204 only on success |
| 5 | Writer/BigQuery outage queues and recovers | **Blocked** | Requires live Pub/Sub push/redelivery |
| 6 | Bounded retry and terminal DLQ | **Blocked** | OpenTofu validates retry/attempt policy; live attempts and DLQ unobserved |
| 7 | Duplicate delivery preserves ID and deduplicates | **Blocked** | Contract and prior M011 evidence pass; M012 Pub/Sub redelivery not observed |
| 8 | No correctness-critical post-request work | Pass | All publish results and sink append result complete before HTTP return |
| 9 | Applied runtime IAM is keyless and least privilege | **Blocked** | No key resources and scoped bindings in OpenTofu; live policy/key inventory unobserved |
| 10 | Infrastructure is repeatable, EU, secret-safe, immutable | Pass | OpenTofu fmt/init/validate, environment isolation, digest validation, GCS backend, WIF workflow |
| 11 | Warm collector replacement and writer scaling | **Blocked** | Configuration passes; Cloud Run replacement unobserved |
| 12 | Monitoring receives and routes actionable signals | **Blocked** | Application metrics and provider alert policies exist; no live time series/notification |
| 13 | EU all-protocol Cloud Run-to-BigQuery proof | **Blocked** | Credential-free path passes; owner authentication prevents apply |
| 14 | Rollback and teardown protect production data | **Blocked** | Runbooks/destroy safeguards exist; no live revisions/resources to exercise |
| 15 | Local quality and container/infrastructure gates | Pass | gofmt, test, race, build, vet, Docker, tofu, and workflow YAML pass |

## Completed artifacts

- Request-scoped architecture decision
- Pub/Sub v2 confirmed publisher and push writer with fakes/tests
- Default, collector, and writer runtime modes
- All-protocol credential-free request integration tests
- Prometheus publish/push and last-delivery metrics
- Keyless OpenTofu Cloud Run/Pub/Sub/BigQuery/IAM/Secret Manager/Monitoring/budget stack
- WIF Artifact Registry digest deployment workflow
- Bootstrap, rollback, teardown, alert, cost, incident, DNS, and soak runbooks
- Live proof plan and exact access evidence

## Unblock condition

1. Reauthenticate a GCP owner/deployment identity for the approved project without sharing credentials in chat.
2. Bootstrap WIF/state/Artifact Registry and apply an isolated EU proof plan.
3. Execute every check in `docs/operations/gcp-proof.md`, including outage, redelivery, DLQ, replacement, alerts, rollback, and teardown.
4. Obtain explicit production DNS/cutover approval, execute the runbook, complete the soak, and retire historical Render files in a dedicated final story.
5. Repeat this judgment with provider evidence.
