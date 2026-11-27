# GCP request-scoped delivery

Status: accepted for the optional GCP-native deployment profile (M012).

## Context

The default Straumheim engine acknowledges input after placing a `Record` in process memory, then a background goroutine batches sink writes. That remains the minimal self-hosted profile, but it cannot make a durable acceptance claim across process replacement. Render's always-available CPU suited that design; adding a managed queue changes the premise.

The GCP profile must be safe with Cloud Run request-based CPU allocation. Correctness may not depend on work continuing after an HTTP response.

## Decision

Deploy one immutable image in two explicit modes:

```text
client -> public collector -> Pub/Sub -> authenticated push -> private writer -> BigQuery
```

### Collector boundary

1. An input parses and normalizes the request into canonical `Record` values.
2. `Ingest` JSON-serializes each complete Record, including `received_at` for transport.
3. It publishes each message to the configured Pub/Sub topic with `record_id` and `protocol` attributes.
4. It calls every publish result's synchronous `Get` before returning.
5. The input returns success only when all messages are confirmed. A timeout or publish error returns HTTP 500; the client did not receive a durable-acceptance acknowledgement.

Publishing multiple Records is not atomic. If result N fails after prior results succeed, the request fails and a client retry can duplicate earlier Records. IDs are generated before publish and remain the deduplication key.

### Writer boundary

1. Cloud Run IAM rejects callers without `roles/run.invoker` before application code.
2. The handler decodes the standard Pub/Sub push envelope and canonical Record JSON.
3. It validates required identity/timestamps and writes the Record through the configured BigQuery sink.
4. It returns 204 only after `AppendResult.GetResult` confirms the append.
5. BigQuery or transport failure returns 500. Pub/Sub retains and redelivers the message.
6. Malformed envelopes/Records return 400. Pub/Sub treats every non-success response as failed delivery; bounded attempts move permanent messages to the configured dead-letter topic.

The default BigQuery stream is at-least-once. Redelivery can create raw duplicate rows with one ID. Analytical queries deduplicate with `ROW_NUMBER() OVER (PARTITION BY id ORDER BY received_at DESC)`.

## Retry and dead-letter policy

- Subscription retry: exponential backoff with explicit minimum and maximum values.
- Retention: explicit and longer than the maximum supported outage objective.
- Dead-letter attempts: explicit within Pub/Sub's supported range.
- Dead-letter topic has its own subscription so messages remain inspectable.
- Logs include message ID, Record ID when decoded, delivery attempt, failure class, and error; never payload or credentials.
- No application retry loop runs after a push request. Pub/Sub owns scheduling and durability.

## Runtime and batching

The collector and writer do no correctness-critical work after their requests finish. Push begins with one Pub/Sub message per request, and the writer performs one Storage Write append per message. This favors a simple durable contract at expected low volume. Future batching must keep all push requests open and unacknowledged until their shared append completes; cross-request fire-and-forget batching is prohibited.

Collector and writer scale independently. Start with two warm collector instances for the accepted availability/latency target and a zero-minimum writer unless live evidence justifies different values.

## Identity boundary

- Collector runtime identity: `pubsub.publisher` on the canonical topic only.
- Writer runtime identity: dataset-scoped BigQuery metadata/create/write permissions only.
- Push identity: `run.invoker` on the writer only.
- Pub/Sub service agent: token creation and dead-letter forwarding permissions required by the authenticated push/DLQ features.
- Deployment identity: separate from runtime identities.
- No downloaded service-account JSON is used.

Configuration contains resource identifiers, not private credentials. Cloud Run obtains ADC from its service identity.

## Compatibility

Empty/default runtime mode keeps the existing in-memory buffer and configured sinks. Pub/Sub is optional and only required by `collector` mode. Direct sinks remain valid for best-effort single-binary deployments.

## Consequences

Advantages:

- acknowledged events survive Cloud Run replacement;
- BigQuery outages become visible Pub/Sub backlog rather than acknowledged loss;
- no host, queue server, persistent disk, or static cloud key;
- request-based Cloud Run billing is safe.

Costs:

- GCP profile has managed Pub/Sub and two Cloud Run services;
- raw delivery is at-least-once;
- per-message writes trade batching efficiency for a clear first production contract;
- public availability is coupled to Pub/Sub publish availability, intentionally failing closed.
