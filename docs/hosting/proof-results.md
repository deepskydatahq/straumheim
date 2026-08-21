# Render proof results

- **Status:** live free-tier proof passed; notification receipt and paid Starter behavior remain unconfirmed
- **Execution window:** 2026-08-21 04:51–05:21 UTC
- **Service:** `srv-da3tio740ujc73cbsqg0`, `https://straumheim-proof.onrender.com`
- **Region/plan:** Frankfurt / Free
- **Normal revision:** commit `d6a16cd3e648fb9dcb21e34fba85d4c6cbb1b850`

The workspace had no payment method, so the official Blueprint correctly rejected the Starter plan with `need_payment_info`. A temporary Free service was created through Render CLI v2.24.0 to test the shared deployment, health, restart, logs, TLS, and rollback mechanisms without incurring compute cost. Free does not support an explicit `maxShutdownDelaySeconds`; this proof therefore does not replace a final Starter smoke test.

## Access and asset validation

| Dependency | Result |
|---|---|
| Render CLI/workspace | Pass — device authorization completed for Timo's workspace |
| Git repository | Pass — Render cloned the public mission branch at the requested commits |
| Secret config | Pass — `config.yaml` mounted at `/etc/secrets/config.yaml` and selected with `STRAUMHEIM_CONFIG` |
| Container build | Pass — Render BuildKit build used a 911 kB context; local context is 1.055 MB after `.dockerignore` |
| Render Blueprint schema | Pass for committed Starter Blueprint; temporary Free variant passes only after removing the paid-only shutdown-delay field |
| Notification destination | Pending operator confirmation — service uses workspace default failure notification setting |
| External database sink | Not used; stdout persisted in Render logs was the non-production proof sink |

Local Docker tests also verified both supported config paths:

- image default: `/etc/straumheim/config.yaml`
- managed-host override: `STRAUMHEIM_CONFIG=/etc/secrets/config.yaml`

## Baseline deployment and event delivery

Initial deploy `dep-da3tiov40ujc73cbssq0`:

- build started `04:51:47Z` and became live `04:52:41Z` (about 54 seconds);
- Render built commit `d6a16cd3e648fb9dcb21e34fba85d4c6cbb1b850`;
- HTTPS certificate verification passed (`curl` verify result 0);
- `GET /health` returned `{"status":"ok"}`;
- webhook proof `m008-render-20260821T045304Z` returned event ID `01a022aa-5a3f-7cd3-a634-2120dcad8e72`;
- at `04:53:04Z`, Render logs contained a stdout-sink JSON record with the same proof ID and event ID.

This proves public HTTPS collection, secret-file configuration, timer/buffer delivery, and centralized logs on Render.

## Running-instance health recovery

A disposable branch (deleted after the test) made `/health` return 503 two minutes after process start. Deployment `dep-da3tk261egvs73arl1j0` became live at `04:55:21Z`.

Observed first cycle:

| Event | UTC | Observation |
|---|---|---|
| Last observed healthy response | 04:57:09 | HTTP 200 |
| Application reports unhealthy | 04:57:14 | HTTP 503 |
| Instance removed/unavailable | 04:57:29 | Public endpoint HTTP 502, 15 seconds after first observed 503 |
| SIGTERM/drain | 04:58:10 | Logs show `shutdown signal received`, `flushing pipeline`, `shutdown complete` |
| Replacement healthy | 04:58:21 | HTTP 200 |

The single Free instance self-healed without SSH, but public health was unavailable for approximately **52 seconds** (04:57:29–04:58:21). The disposable code failed again after two minutes and started a second recovery cycle, confirming repeatability. The service was then restored to the normal branch.

**Implication:** Render automates recovery, but one Starter/Free instance does not provide continuous availability during replacement. Multiple healthy instances would be required to hide this interval and would increase cost. Graceful shutdown completed for an empty/fast stdout buffer; a blocked destination could still exceed the platform window and lose memory.

## Unhealthy candidate deployment

A second disposable commit (`763c9881874a94ebb48b0fc357f737665e3a6934`, branch deleted) returned 503 immediately from `/health`.

- candidate deploy `dep-da3tnr3ncjis73ahsp30` started `05:02:36Z`;
- candidate remained `update_in_progress` while the old revision served HTTP 200 throughout the polling window;
- Render marked the candidate `update_failed` at `05:18:15Z` (about 15 minutes 39 seconds);
- the prior healthy deploy stayed live;
- after failure, webhook proof `m008-after-failed-deploy-20260821T051846Z` returned event ID `01a022c1-e319-75ef-905f-0ee63970e9f2`, and the matching stdout record appeared in Render logs at `05:18:47Z`.

This validates health-gated deployment and failure isolation. It also shows that a bad candidate takes roughly 15 minutes to be declared failed under Render's fixed deployment-health window.

## Rollback

A harmless second healthy revision (`d813fbed06037b1bcf15c176b5c162ce6293a7eb`, disposable branch deleted) was deployed. The Render rollback API then targeted normal deploy `dep-da3tn3740ujc73cc6u70`.

- rollback deploy `dep-da3u0f3tqb8s73fktveg` started `05:21:00Z` with trigger `rollback`;
- it restored commit `d6a16cd3e648fb9dcb21e34fba85d4c6cbb1b850`;
- rollback became live at `05:21:22Z` while public health remained 200;
- webhook proof `m008-after-rollback-20260821T052123Z` returned event ID `01a022c4-482d-7614-b16d-e4fcfacb5d04`;
- the matching record appeared in logs at `05:21:24Z`.

Rollback passed and did not require a rebuild.

## Drill status

| Drill | Status | Evidence |
|---|---|---|
| Frankfurt HTTPS health | Pass | Public URL, verified TLS, HTTP 200 |
| Synthetic event to persisted debug logs | Pass | Matching response/log IDs above |
| Unhealthy candidate retains healthy deploy | Pass | 15-minute failed candidate; serving revision stayed 200 and accepted an event |
| Running health failure removes/restarts instance | Pass | 503 → traffic removal/502 → SIGTERM/drain → replacement/200 |
| Graceful shutdown | Pass for fast proof sink | Three log sequences include shutdown, flush, and completion |
| Rollback | Pass | Rollback trigger restored normal commit and event path |
| Failure notification delivery | Pending | Workspace default configured; operator must confirm email receipt |
| Free-tier cold start after idle | Pass with latency caveat | After more than 16 idle minutes, `/health` returned 200 in 13.297 seconds; immediate webhook delivery also passed |
| External sink outage visibility | Not run | Requires disposable external destination credentials; M009/M010 capture the known gap |
| Paid Starter behavior | Not run | Workspace needs payment information; committed Blueprint remains Starter |

## Free-tier cold start

After more than 16 minutes without public requests, a request started at `05:38:00.008Z`. Render started Straumheim at `05:38:04Z`, and `/health` returned HTTP 200 after **13.297 seconds** at `05:38:13.311Z`. TLS verification passed.

An immediate webhook with proof ID `m008-after-cold-start-20260821T053823Z` returned event ID `01a022d3-d569-7ded-9e1c-1f3c9d681839`; the matching stdout record appeared in Render logs at `05:38:23Z`. No event was lost in this observed cold-start path because Render queued the request until the container was ready. Browser beacons with shorter client lifetimes can still abandon a 13-second request. Paid Starter avoids Free's idle spin-down.

## Remaining actions

1. Confirm whether Render failure/unhealthy emails reached `timo@partnerwithpropel.com`.
2. Optionally add payment information and repeat a short Starter smoke test for explicit 30-second shutdown configuration and no idle spin-down.
3. Delete the proof service and verify no paid resources remain.
