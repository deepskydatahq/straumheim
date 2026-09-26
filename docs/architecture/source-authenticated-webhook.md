# Authenticated source webhook

Status: local/synthetic implementation only; not configured or deployed in production.

The generic `/webhook` route accepts arbitrary JSON and does not assign a trusted `Record.Source`. The source webhook adds one fixed, server-configured route for one source. Its configuration owns path, source, key ID/secret, vendor, schema, version, and clock-skew bound. Event payload fields cannot change those values.

A client sends:

- `X-Straumheim-Key-Id`
- `X-Straumheim-Timestamp` as RFC 3339
- `X-Straumheim-Event-Id` as UUID
- `X-Straumheim-Signature: sha256=<hex>`

The signature is HMAC-SHA256 over:

```text
timestamp + "\n" + event_id + "\n" + sha256_hex(exact_request_body)
```

The route accepts at most 64 KiB, requires JSON, rejects unknown key IDs, bad signatures, invalid event IDs, and timestamps outside the configured skew. It also enforces the closed Propel event field set, enums, UUIDs, timestamps, count limits, payload schema version, and header/payload event-ID equality before ingestion, then preserves the producer event ID as the Straumheim Record ID. At-least-once delivery may still duplicate that ID; governed consumers deduplicate and separately measure duplicate delivery.

The source route intentionally leaves Record IP, user-agent, and referrer empty. Request logging omits remote address for `/source/` paths. Cloud-provider security logs may still contain platform-level network metadata and remain governed by their separate restricted access and retention policy.

Example disabled configuration is in `config.example.yaml`. The secret comes from environment substitution and must be provisioned independently for client and collector. Do not commit it, log it, send it in an event, or expose it to a model.

The GCP root keeps the source disabled by default. Enabling it requires a key ID, an existing Secret Manager resource ID, and an exact numeric secret version; OpenTofu grants only the collector service account access and mounts that version as `PROPEL_HARNESS_SOURCE_SECRET`. `latest` is refused. The secret value is provisioned and rotated outside OpenTofu state.

Production enablement requires separate review of source registration, secret provisioning/rotation, collector deployment, raw-table retention, BigQuery governed-model access, canary, monitoring, and rollback.
