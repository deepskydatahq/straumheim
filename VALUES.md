# Straumheim Value Ladder

> Self-hosted event data pipeline — from a Docker container to a complete data collection platform where every event lands in infrastructure you own.

**Last Updated:** March 2026

---

## How to Read This

Straumheim's value story is an ordered progression of **15 levels**. Each level is independently valuable, buildable, and compounds toward the end state: a production-grade event data platform that replaces commercial CDPs for any product that wants to own its event pipeline.

Levels build on each other. Lower levels are prerequisites for higher ones.

**Status key:**
- **shipped** — Working in the current codebase
- **building** — Active development
- **designed** — Architecture defined, not yet built
- **planned** — Scope understood, design pending
- **future** — Requires capabilities not yet in place

---

## Status Summary

| Level | Name | Tier | Status |
|-------|------|------|--------|
| L01 | Multi-Protocol Collection | Core Pipeline | **shipped** |
| L02 | Record Normalization | Core Pipeline | **shipped** |
| L03 | Batch Delivery | Core Pipeline | **shipped** |
| L04 | Warehouse Sink | Destination Coverage | **shipped** |
| L05 | File & Object Storage | Destination Coverage | **shipped** (JSONL local; S3/Parquet planned) |
| L06 | Columnar Analytics | Destination Coverage | planned |
| L07 | Schema Validation | Data Quality | planned |
| L08 | Deduplication | Data Quality | planned |
| L09 | Data Contracts | Data Quality | future |
| L10 | Durable Buffering | Reliability at Scale | planned |
| L11 | Backpressure & Retry | Reliability at Scale | planned |
| L12 | Horizontal Scale | Reliability at Scale | future |
| L13 | Event Routing | Data Platform | future |
| L14 | Event Replay | Data Platform | future |
| L15 | Protocol Ecosystem | Data Platform | future |

---

## Tier 1: Core Pipeline (L01–L03)

*Docker run, events flow.*

These levels deliver value the moment someone runs `docker run straumheim`. The input is HTTP traffic from trackers; the output is events in a destination.

---

### L01: Multi-Protocol Collection
> "Point your Snowplow tracker, webhook, or pixel at one endpoint — it handles all of them."

**Status:** shipped
**Scope:** v1

**What it delivers:** HTTP endpoints that speak Snowplow tracker protocol (GET pixel + POST tp2), accept generic JSON webhooks, and collect pixel tracking hits. Each protocol gets its own route prefix and wire format parser. Snowplow cookie handling included.

**Why it matters:** Protocol handling is the first barrier to self-hosted collection. Snowplow trackers are the largest open-source tracker ecosystem — compatibility means instant access to battle-tested client SDKs (JavaScript, iOS, Android, GTM) without building custom trackers. Webhook and pixel support covers everything else.

---

### L02: Record Normalization
> "Every event becomes a Record — same structure, same metadata, regardless of where it came from."

**Status:** shipped
**Scope:** v1

**What it delivers:** A common `Record` structure with UUIDv7 IDs, collector and device timestamps, protocol tag, source identifier, raw payload, and flattened payload. JSON flattening converts nested objects to dot-notation keys for warehouse compatibility. Collector metadata (IP, user agent, referer) is extracted from every request.

**Why it matters:** Without normalization, every downstream component needs to understand every input protocol. The Record is the contract between collection and delivery — it decouples inputs from sinks completely. Add a new input? It produces Records. Add a new sink? It consumes Records. Nothing else changes.

---

### L03: Batch Delivery
> "Events flow from buffer to destination — batched by count or time, fan-out to multiple sinks."

**Status:** shipped
**Scope:** v1

**What it delivers:** An in-memory buffer (Go channels) that accumulates records and flushes them to configured sinks by batch count or time interval. Each sink receives every record. Sinks operate independently — a slow Postgres doesn't block stdout output.

**Why it matters:** Naive event-at-a-time delivery is 10-100x slower than batching for database sinks. The buffer + batch flush pattern is what makes a single binary performant enough for production workloads. Fan-out to multiple sinks means you get warehouse storage and debug output from the same pipeline.

---

## Tier 2: Destination Coverage (L04–L06)

*Events land where you need them.*

These levels expand where events can be delivered. Each destination is a sink implementation behind a clean interface — adding one never changes the core pipeline.

---

### L04: Warehouse Sink
> "Events in Postgres, queryable with SQL, auto-schema included — no DDL management."

**Status:** shipped
**Scope:** v1

**What it delivers:** A Postgres sink using `COPY FROM STDIN` for batch ingestion (fastest possible Postgres write path). Auto-creates the events table on first write. Auto-adds columns when new fields appear in records. Invalid records go to a separate table for debugging, never silently dropped.

**Why it matters:** Postgres is the destination 80% of users want. Events in Postgres means instant access to SQL, dbt, Metabase, Grafana — the entire SQL ecosystem. Auto-schema is the key DX win: just send events, new fields become columns automatically. No migration files, no ALTER TABLE, no coordination with engineering.

---

### L05: File & Object Storage
> "JSONL files locally, Parquet on S3 — archive everything, query with DuckDB."

**Status:** shipped (JSONL local); S3 and Parquet planned for v2
**Scope:** v1 (JSONL local), v2 (S3, Parquet)

**What it delivers:** A file sink writing JSONL (one JSON object per line) to local filesystem, with time-based or size-based file rotation. S3-compatible output (MinIO, AWS S3, R2) as fast-follow. Parquet format for efficient analytical queries.

**Why it matters:** File output is the zero-infrastructure destination — no database needed, just a volume mount. JSONL files are immediately queryable with DuckDB, jq, or any line-oriented tool. S3 + Parquet becomes the cost-effective archive: store everything for pennies, query on demand. This is the "I want all my events but I'll query them later" path.

---

### L06: Columnar Analytics
> "ClickHouse for real-time queries, BigQuery for cloud-scale — same events, different engines."

**Status:** planned
**Scope:** v2

**What it delivers:** Sink implementations for ClickHouse (batch insert, MergeTree tables) and BigQuery (streaming inserts or batch load). Each optimized for the destination's preferred ingestion pattern.

**Why it matters:** Postgres handles millions of events well. Billions of events need columnar storage. ClickHouse gives sub-second queries over event data at massive scale. BigQuery gives serverless analytics for GCP-native teams. These sinks unlock Straumheim for high-volume products without changing the collection layer.

---

## Tier 3: Data Quality (L07–L09)

*Trust what's in the warehouse.*

These levels add validation, deduplication, and contracts to ensure the events landing in destinations are correct, complete, and trustworthy.

---

### L07: Schema Validation
> "JSON Schema validation for events that need it — opt-in, cached, fast."

**Status:** planned
**Scope:** v2

**What it delivers:** JSON Schema validation against a schema registry (local filesystem or S3). Schemas are addressed by vendor/name/version (e.g., `com.example/signup/v1.0`). Validated with in-memory LRU cache. Invalid records are tagged, not dropped — they flow to a separate table or stream.

**Why it matters:** As event volume grows, so does the risk of malformed data. Schema validation catches broken tracker implementations, API contract violations, and payload corruption before they pollute the warehouse. Opt-in means simple setups aren't burdened; teams add validation as their data quality requirements mature.

---

### L08: Deduplication
> "Same event, delivered once — even if the tracker retries."

**Status:** planned
**Scope:** v2

**What it delivers:** Record-level deduplication using UUIDv7 IDs. In Postgres, this is `ON CONFLICT (id) DO NOTHING`. For other sinks, a configurable dedup window using a bloom filter or LRU set.

**Why it matters:** Network retries, tracker bugs, and mobile reconnections all produce duplicate events. Without dedup, every metric is slightly inflated. UUIDv7 makes dedup natural — the ID is the dedup key, generated at the source, monotonically ordered for efficient conflict resolution.

---

### L09: Data Contracts
> "Schema evolution rules — what's allowed to change, what breaks, and who gets notified."

**Status:** future

**What it delivers:** Schema compatibility checks (forward-compatible, backward-compatible, full-compatible) that run when new schema versions are registered. Breaking change detection that can warn or block. Integration with CI/CD to validate schema changes before deployment.

**Why it matters:** Schema validation tells you if an event is valid now. Data contracts tell you if a schema change will break downstream consumers. As the number of event producers grows, contracts prevent the "someone changed the payload and broke the dashboard" failure mode. This is the shift from reactive data quality to proactive data governance.

---

## Tier 4: Reliability at Scale (L10–L12)

*Don't lose events, even under pressure.*

These levels replace the default in-memory buffer with durable alternatives and add the reliability guarantees needed for production workloads where event loss is unacceptable.

---

### L10: Durable Buffering
> "SQLite WAL for single-node durability — events survive restarts without Kafka."

**Status:** planned
**Scope:** v2

**What it delivers:** A SQLite WAL-based buffer that persists events to disk before delivery. On crash or restart, undelivered events are recovered and retried. Zero external dependencies — SQLite is embedded in the binary.

**Why it matters:** The in-memory buffer is fast but volatile — a process crash loses buffered events. For products where event loss is unacceptable (billing events, compliance tracking, revenue attribution), durable buffering is the minimum bar. SQLite WAL gives durability without the operational burden of running Kafka or Redis.

---

### L11: Backpressure & Retry
> "When a destination is down, events queue up. When it's back, they drain. Nothing is lost."

**Status:** planned
**Scope:** v2

**What it delivers:** Per-sink backpressure that pauses consumption when a destination is unhealthy. Exponential backoff retry for transient failures. Dead letter queue for permanently failed records. Health status per sink exposed via the metrics endpoint.

**Why it matters:** Destinations go down. Postgres runs out of connections. S3 has a regional outage. Without backpressure, events are dropped or the pipeline crashes. With it, the buffer absorbs the spike, retries resume delivery when the destination recovers, and permanently failed records go to a dead letter queue for investigation.

---

### L12: Horizontal Scale
> "Multiple Straumheim instances behind a load balancer — stateless collection, shared nothing."

**Status:** future

**What it delivers:** Stateless collection endpoints that can run behind any HTTP load balancer. Shared buffer backends (NATS, Redis, Kafka) for multi-instance coordination. Consistent hashing for destination partitioning.

**Why it matters:** A single Straumheim instance handles thousands of events per second. But when you need tens of thousands — global products, high-traffic marketing sites, IoT event streams — horizontal scale means adding instances, not replacing the tool. The upgrade path from single binary to clustered deployment should be a config change, not an architecture migration.

---

## Tier 5: Data Platform (L13–L15)

*From pipeline to platform.*

These levels transform Straumheim from a collection tool into a programmable event data platform. Events aren't just collected and delivered — they're routed, replayed, and extended.

---

### L13: Event Routing
> "Send pageviews to ClickHouse, billing events to Postgres, everything to S3 — content-based routing."

**Status:** future

**What it delivers:** Routing rules that direct records to specific sinks based on event content — protocol, source, schema, payload fields. Filter expressions in the config file. Conditional fan-out: some events go everywhere, others go to specific destinations.

**Why it matters:** Not all events belong in all destinations. Pageviews generate volume that belongs in a columnar store. Billing events need the transactional guarantees of Postgres. Debug events should go to files, not the warehouse. Routing makes each destination receive exactly the events it should — reducing storage costs and query noise.

---

### L14: Event Replay
> "Re-deliver last Tuesday's events to the new ClickHouse sink — from the archive, not the source."

**Status:** future

**What it delivers:** The ability to replay archived events (from S3/file storage) through the sink layer. Select events by time range, protocol, source, or schema. Replay to a specific destination without affecting others.

**Why it matters:** Replay turns the archive from cold storage into an active asset. Added a new destination? Replay historical events into it. Fixed a schema bug? Re-process affected events. Changed a transformation? Replay and overwrite. Without replay, adding a new sink means you only get data from now on. With it, you get the complete history.

---

### L15: Protocol Ecosystem
> "Segment-compatible /v1/track, CloudEvents, custom protocols — extend collection without forking."

**Status:** future

**What it delivers:** Additional input protocols: Segment-compatible API (`/v1/track`, `/v1/identify`, `/v1/page`), CloudEvents, and a protocol plugin interface for custom wire formats. Each protocol implements the Input interface — the rest of the pipeline is unchanged.

**Why it matters:** Snowplow, webhooks, and pixels cover most use cases, but the event ecosystem is fragmented. Segment compatibility unlocks migration from Segment without changing client code. CloudEvents support connects to serverless and event-driven architectures. The protocol plugin interface makes Straumheim extensible by the community — new inputs without core changes.

---

## The End State

The 15 levels build toward a **self-hosted event data platform**: a production-grade system that collects events from any source, validates and deduplicates them, delivers to any destination, and scales from a side project to a serious product.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    EVENT DATA PLATFORM                                   │
│                                                                         │
│   Every event × every protocol × every destination                      │
│   Validated, deduplicated, durably buffered, routable                   │
│                                                                         │
│   Replaces:                                                             │
│   - Segment ($$$) with a $5/mo VPS                                      │
│   - Snowplow OSS (Kafka + Kinesis) with a single binary                 │
│   - Custom scripts (fragile) with production-grade infrastructure       │
│                                                                         │
│   Built progressively:                                                  │
│   L01-L03  →  The pipeline (collect, normalize, deliver)                │
│   L04-L06  →  The destinations (Postgres, S3, ClickHouse)               │
│   L07-L09  →  The quality (schemas, dedup, contracts)                   │
│   L10-L12  →  The reliability (durable buffer, retry, scale)            │
│   L13-L15  →  The platform (routing, replay, protocols)                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

Each level is independently valuable. L01–L03 alone ("collect events, put them in Postgres") solves the problem for most small products. But the compounding value of each level makes Straumheim viable for increasingly demanding workloads.

The priority is simple: **build the lowest unshipped level first.**
