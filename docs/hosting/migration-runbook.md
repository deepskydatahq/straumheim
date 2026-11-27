# Hetzner-to-Render migration runbook

Status: **historical, superseded for production by M012**. Render proof resources are de-provisioned. Do not execute this runbook; use [the GCP-native runbook](../operations/gcp-runbook.md). The content remains as rollback/decision evidence from M008.

## Owners

Assign names before execution:

| Role | Responsibility |
|---|---|
| Migration owner | Render deployment, evidence, go/no-go, rollback decision |
| DNS owner | TTL reduction, collector record changes, rollback record |
| Destination owner | Least-privilege credential, event query, delivery validation |
| Alert owner | Receives and responds to Render and delivery alerts |

One person may hold all roles, but every field must be explicit in the change record.

## Hard prerequisites

Do not schedule cutover until:

- [ ] M008-E002-S002 and M008-E002-S003 are complete with live evidence
- [ ] production destination supports encrypted network access from Render Frankfurt
- [ ] each Starter instance can carry full current event volume with measured margin while its peer is unavailable
- [ ] Render service health and deploy-failure email/Slack notifications are tested
- [ ] an interim alert exists for `straumheim_records_failed_total` or `sink write failed`
- [ ] destination credentials are least privilege and separately rotatable
- [ ] existing Hetzner deployment/config/traffic baseline is documented
- [ ] owner accepts that M009 is not yet implemented and acknowledged events can be lost on restart/sink failure
- [ ] rollback authority and observation-window length are approved

If any prerequisite is false, stop. Do not interpret this runbook as approval to continue.

## Phase 1: Prepare without traffic changes

1. Reduce collector DNS TTL to 300 seconds at least one prior TTL window before cutover.
2. Record current DNS values and Hetzner public endpoint.
3. Export the effective Hetzner Straumheim configuration **without secrets** for comparison.
4. Record the exact production image digest or source commit currently running.
5. Verify the latest main commit passes CI and corresponds to the intended release.
6. Apply `render.yaml` to create two Frankfurt Starter instances. For production, rename `straumheim-proof` deliberately and review the Blueprint diff; do not silently reuse proof infrastructure.
7. Upload production `config.yaml` as a Render secret file. It appears at `/etc/secrets/config.yaml`. Never put its contents in Git, ticket text, logs, or screenshots.
8. Set production destination credentials as Render secret environment variables or inside the secret file. Prefer environment substitution and least-privilege credentials.
9. Keep the generated `onrender.com` URL enabled during validation.
10. Configure failure notifications and a cost/budget alert before sending events.

### Configuration review

Confirm:

- `server.host` is `0.0.0.0` and port is `8080`;
- `PORT=8080` and `STRAUMHEIM_CONFIG=/etc/secrets/config.yaml`;
- CORS origins and Snowplow cookie domain match the intended collector domain;
- no local file sink is enabled;
- destination TLS, database/table, credentials, and timeouts are correct;
- stdout logging does not expose unacceptable event payload data in production;
- `numInstances` is 2, each instance can handle full traffic during peer recovery, and both fit the expected $14/month compute budget;
- buffer capacity/flush settings fit measured per-instance volume and shutdown window.

## Phase 2: Validate the generated Render endpoint

1. Verify `/health` over Render-managed HTTPS.
2. Send uniquely tagged webhook, pixel, and Snowplow test events for each enabled input.
3. Confirm each response and query the destination for the same unique IDs.
4. Inspect centralized logs and `/metrics` for received, delivered, failed, buffer, and flush behavior.
5. Run the approved proof failure and rollback checks against this release if its config differs materially from proof.
6. Let the service run without production DNS for an agreed soak period while sending periodic synthetic events from an external uptime service.

**Go gate:** continue only if all test events arrive exactly as expected, failure alerts route to the owner, no unexplained restart occurs, and resource use remains comfortably below limits.

## Phase 3: DNS cutover

1. Announce the change window and freeze unrelated collector/deployment changes.
2. Capture pre-cutover Hetzner and destination counters.
3. Add the production custom domain to Render and complete ownership verification before changing traffic.
4. Update DNS to the Render target exactly as Render instructs.
5. Keep Hetzner running and unchanged. Do not stop it.
6. From multiple networks, verify certificate, `/health`, and all enabled event endpoints.
7. Send uniquely tagged canary events through the custom domain and verify destination delivery.
8. Compare received/delivered/failure rates and destination counts at 5, 15, 30, and 60 minutes.

Avoid deliberate dual writes from one tracker unless downstream deduplication is proven. DNS propagation may naturally split traffic; use unique IDs and destination queries to understand it.

## Immediate rollback triggers

Restore the recorded Hetzner DNS value if any occurs:

- custom-domain TLS or routing remains invalid after the agreed threshold;
- `/health` or enabled collector endpoints repeatedly fail;
- test events are acknowledged but absent from the destination;
- `sink write failed` appears repeatedly;
- failed delivery counter increases without recovery;
- CORS/cookie behavior breaks tracking clients;
- Render restart loop or notification occurs;
- latency/error rate exceeds the agreed baseline; or
- operator cannot explain a material count discrepancy.

After DNS rollback, verify Hetzner endpoint and destination delivery, preserve Render logs/events, and open a product story with evidence. Do not retry in the same window without understanding the cause.

## Phase 4: Observation and Hetzner retirement

1. Keep Hetzner available for at least seven days, or longer than the maximum accepted DNS/cache and incident-detection window.
2. Operate alert-driven: respond to notifications; do not add routine SSH or manual restart checks.
3. Review cost and resource metrics after 24 hours and at the end of the observation window. This is migration validation, not a permanent checklist.
4. Verify no DNS traffic reaches Hetzner before retirement.
5. Take only the backup/export required by retention policy; securely remove copied secrets.
6. Revoke Hetzner-only credentials and firewall rules.
7. Delete the VM, volumes/snapshots not required by retention policy, and obsolete Caddy/Docker operational automation.
8. Raise DNS TTL after confidence is established.
9. Record final Render service, domain, release, cost baseline, alert owner, and rollback outcome.

## Normal operations: alerts, not checks

| Alert/event | Automated platform action | Operator action |
|---|---|---|
| Candidate deploy fails | Old Render instance remains serving | Read build/deploy event; fix or roll back source; no SSH |
| Running service unhealthy | Render removes traffic, restarts, notifies | Inspect logs/events; verify event path; escalate repeated failures |
| Service recovers | Render returns healthy instance to service | Confirm canary delivery only if an incident fired |
| `sink write failed` / delivery failures | Straumheim currently only logs/counts | Treat as data incident; restore destination; assess loss; M009 is the permanent fix |
| CPU/memory saturation | Depends on configured external/native alert | Reduce load/scale or investigate; do not wait for crash |
| Cost threshold | Billing alert | Inspect resource/egress change and cap unexpected use |
| Render platform incident | Provider status/automation | Follow provider incident, use DNS rollback only if threshold is breached |

No daily/weekly uptime check, package update, VM reboot, Caddy renewal check, or routine SSH session is part of normal operation.

## Secrets rotation

1. Create replacement destination credential.
2. Save it in Render without deleting the old credential.
3. Deploy and verify canary delivery.
4. Revoke the old credential.
5. Record rotation date and owner, not the value.

## Related evidence

- [Requirements](requirements.md)
- [Platform comparison](platform-comparison.md)
- [Decision](decision.md)
- [Proof plan](proof-plan.md)
- [Proof results](proof-results.md)
- M009 durable delivery mission
- M010 operational health mission
