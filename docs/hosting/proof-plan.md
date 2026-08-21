# Render proof plan

Purpose: verify the M008 decision in a disposable Frankfurt Starter service. This plan does **not** authorize production traffic or DNS changes.

## Prerequisites

- Render account/workspace with billing for one Starter service
- GitHub access to `deepskydatahq/straumheim`
- email notification enabled; optionally a connected Slack workspace
- temporary branch if failure-drill code is required
- local `curl`, Git, and access to Render service events/logs

Estimated proof cost is prorated from $7/month; delete the service after evidence capture.

## 1. Create the disposable service

1. In Render, create a Blueprint from this repository’s `render.yaml`.
2. Confirm before applying:
   - service: `straumheim-proof`
   - type/runtime: web/Docker
   - plan/region: Starter/Frankfurt
   - one instance, no persistent disk
   - health path: `/health`
   - shutdown delay: 30 seconds
   - auto-deploy: after CI checks pass
   - `PORT=8080`
   - `STRAUMHEIM_CONFIG=/etc/secrets/config.yaml` (overrides the image default `/etc/straumheim/config.yaml`)
3. Add a Render **secret file** named `config.yaml` with the contents of `deploy/render/config.proof.example.yaml`. Render mounts it at `/etc/secrets/config.yaml`. Do not add destination credentials for this proof.
4. Configure workspace/service notifications to **Only failure notifications** for email and optionally Slack.
5. Apply the Blueprint. Record service ID, region, deployed commit and build/deploy event timestamps in `proof-results.md`.

The Git-backed Docker build is tied to a commit/build artifact. Do not switch the proof to a mutable `:latest` image. If using a registry-backed service instead, pin an OCI digest.

## 2. Baseline HTTPS and delivery

Set the generated URL without a trailing slash:

```bash
export PROOF_URL=https://straumheim-proof.onrender.com
curl --fail-with-body --show-error "$PROOF_URL/health"

curl --fail-with-body --show-error \
  -H 'Content-Type: application/json' \
  -d '{"event":"m008_proof","proof_id":"m008-<UTC timestamp>"}' \
  "$PROOF_URL/webhook"
```

Record:

- health response and UTC timestamp;
- webhook response ID;
- the JSON record with the same `proof_id` and response ID in Render logs;
- the running deploy’s commit/artifact identity;
- TLS issuer/details shown by the browser or `openssl s_client` (do not paste unrelated certificate data).

The stdout sink is acceptable only for this proof because Render persists stdout in platform logs. Production must use an external database/object sink.

## 3. Unhealthy candidate deployment

Goal: prove the healthy deployment remains live when a candidate cannot start or pass health.

1. Keep a terminal polling the current event path and `/health`.
2. On a temporary proof branch, create a disposable commit that makes the candidate fail startup (for example, set the Docker command to a nonexistent executable in the disposable service only). Do not merge it.
3. Trigger the deploy and wait for Render to cancel it.
4. Verify throughout that the previous service still returns 200 and accepts a new uniquely identified synthetic event.
5. Capture the failed deployment event and failure notification with UTC timestamps.
6. Restore the normal command/config and confirm no disposable failure change remains in the repository.

Do not alter the production/default branch to run this drill.

## 4. Running-instance health and restart drill

Straumheim’s current `/health` cannot be toggled and always returns 200 while the router runs. Use a **temporary, unmerged proof branch** that makes `/health` return 503 after a clearly named proof-only environment variable is enabled. Do not add this behavior to production.

1. Deploy the temporary instrumented revision healthy first.
2. Enable the proof failure trigger and record when `/health` first returns 503.
3. Observe Render removing traffic after consecutive failures, restarting the instance after continued failure, and sending an unhealthy-service notification.
4. Disable the trigger or redeploy the normal commit.
5. Record event timing, instance/deploy identity, notification receipt, interruption, and recovery.

If the platform cannot toggle the variable without redeploying, use a disposable proof endpoint/image designed to fail after a timer. Record that this tests platform health recovery but not the production binary.

## 5. Rollback and shutdown evidence

1. Deploy a harmless identifiable proof revision.
2. Use Render **Rollback** on the prior successful deploy.
3. Confirm the prior artifact becomes live and the webhook path still delivers to logs.
4. Inspect old-instance logs for `shutdown signal received`, `flushing pipeline`, and `shutdown complete` to prove SIGTERM reached Straumheim.
5. Note that a missing `shutdown complete`, platform SIGKILL, process crash, or blocked sink can lose the in-memory batch.

## 6. Sink outage visibility

A stdout proof has no external credentials to fail. For the final production-readiness proof, repeat in a separate disposable service using a non-production Postgres/ClickHouse destination, revoke or block connectivity, and verify:

- `sink write failed` appears centrally;
- `straumheim_records_failed_total` increments while the instance remains live;
- `/health` remains 200 today;
- no platform unhealthy alert fires solely from sink failure.

This expected gap motivates M009 and M010. Never use production credentials for the drill.

## 7. Cleanup

1. Export or copy only non-secret evidence into `proof-results.md`.
2. Delete temporary failure branches and ensure no proof-only code was merged.
3. Delete the Render proof service/Blueprint and secret file.
4. Verify billing shows no remaining proof compute.
5. Revoke any temporary credential or notification webhook.

## Evidence standard

Use observed facts only. Redact tokens, secret file contents, internal service IDs if sensitive, notification webhook URLs, and destination credentials. A screenshot alone is insufficient: include UTC timestamp, action, expected behavior, observed behavior, and pass/fail for every drill.
