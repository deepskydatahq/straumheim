# STRAUMHEIM
## Product Vision Document v1

*Lightweight, self-hosted event data pipeline*

**March 2026 | deepskydata**

---

## Vision Statement

> *Every product generates events — clicks, pageviews, signups, purchases.*
> *Most teams pipe them through a vendor they don't control and can't afford. Straumheim collects them — and anyone can run it.*

Straumheim is a **lightweight, self-hosted event data pipeline** — a single Go binary that collects events from any source (Snowplow trackers, webhooks, pixels), normalizes them into a common record, and delivers them to one or more destinations (Postgres, S3, ClickHouse, files).

No Kafka required. No Redis dependency. No vendor lock-in. Point your trackers at it, configure your destinations, and your events flow to infrastructure you own.

Ship it as a Docker container. Configure it with a YAML file. Scale it from a side project to a serious product — the same binary, the same config format, more destinations and durability as you grow.

Part of the `-heim` family alongside Fyrnheim (data transformation).

---

## The Paradigm Shift

### Old Model: Rent Your Event Pipeline

Send your events to a vendor's cloud. Pay per event. Accept their schema. Hope they don't change pricing, deprecate endpoints, or get acquired. When you outgrow them, migrate everything — trackers, schemas, destinations.

The problem isn't the vendor. It's the **dependency**. Your event data — the raw material of every product decision — flows through infrastructure you don't control.

### New Model: Own Your Event Pipeline

Straumheim gives you the same capabilities the vendors sell — multi-protocol collection, schema validation, batch delivery to warehouses — in a single binary you deploy yourself.

The shift is from "rent your pipeline" to "own your pipeline."

An event pipeline should be:
- **Self-hosted:** Run on your infrastructure, your cloud, your terms
- **Minimal:** Single binary, single config file, no orchestration layer required
- **Protocol-agnostic:** Snowplow trackers, generic webhooks, tracking pixels — one collector handles all of them
- **Destination-flexible:** Start with Postgres and a log file, add S3 and ClickHouse when you need them
- **Predictable:** No per-event pricing, no usage surprises, no vendor negotiations

---

## The Problem We Solve

### The Surface Problem

Small-to-medium products need event collection but can't justify the cost or complexity of commercial CDPs. Segment charges per event. Snowplow's open-source collector requires Kafka, Kinesis, or PubSub. Jitsu needs Kafka and Redis. The "simple" option doesn't exist.

### The Deeper Problem

Event infrastructure has a missing middle. You either get:
- **Too simple:** Google Analytics, Plausible — aggregated metrics, no raw events, no custom destinations
- **Too complex:** Snowplow OSS, Jitsu, RudderStack — powerful but require orchestration layers, message queues, and dedicated ops effort
- **Too expensive:** Segment, mParticle — simple to use but $$$$ at scale

There's no "Postgres of event collection" — a boring, reliable tool that does one thing well and runs anywhere.

### Why This Persists

Building an event pipeline from scratch means solving:
- Multi-protocol parsing (every tracker speaks a different wire format)
- Schema validation without blocking delivery
- Batch delivery to warehouses (naive INSERT is 100x slower than COPY)
- Auto-schema evolution (new event fields shouldn't require DDL migrations)
- Buffering and backpressure (spikes shouldn't drop events)
- Cookie handling, IP extraction, user agent parsing

Each of these is a solved problem. But wiring them together into a reliable, production-ready binary is a month of work — and maintaining it is ongoing.

### What Changes Everything

**What if event collection was a single binary?**

What if you could `docker run straumheim` with a YAML config and immediately:
- Collect events from Snowplow trackers, webhooks, and tracking pixels
- Normalize everything into a common record with UUIDs, timestamps, and metadata
- Batch-deliver to Postgres using COPY (fastest possible ingestion)
- Write JSONL files for archival or DuckDB analysis
- Add new destinations by editing config — no code changes

What if the same tool worked for a hobby project and a production SaaS?

That's Straumheim.

---

## Core Belief System

### The Keystone Belief

> **"I need to collect and store my product's events — pageviews, signups, feature usage — but I don't want to pay per-event pricing, manage Kafka, or depend on a vendor for my own data."**

This belief is central because:

1. **It acknowledges the need.** Every product team wants event data for analytics, debugging, and product decisions. Raw events are the foundation.

2. **It acknowledges the cost constraint.** Per-event pricing makes teams afraid to track things. They instrument less, understand less, decide worse.

3. **It acknowledges the complexity constraint.** Running Kafka for a 10K MAU product is absurd. But that's what most open-source collectors require.

4. **It creates the opening.** If collection was a single binary with zero infrastructure dependencies — suddenly every product can have proper event collection.

### The Belief Chain

| # | Belief | Type |
|---|--------|------|
| 1 | Our product generates events that contain signals about user behavior | Observable reality |
| 2 | To make good product decisions, we need access to raw event data | The reframe |
| 3 | Commercial CDPs charge per event, making comprehensive tracking expensive | Pain recognition |
| 4 | Open-source alternatives require Kafka/Redis/PubSub — too complex for our scale | Pain recognition |
| 5 | We've considered building our own, but the edge cases are endless | Exhaustion |
| **6** | **I need event collection without per-event pricing or Kafka** | **KEYSTONE** |
| 7 | If it was a single binary with a config file... | Solution direction |
| 8 | ...that handled Snowplow, webhooks, and pixels out of the box... | Trust requirement |
| 9 | ...then I'd finally own my event pipeline | Product fit |

---

## How Straumheim Works

### Step 1: Deploy

Single Docker container with a YAML config file. No orchestration required.

```bash
$ docker run -d \
    -p 8080:8080 \
    -v ./config.yaml:/etc/straumheim/config.yaml \
    -e POSTGRES_DSN="postgres://user:pass@host/analytics" \
    ghcr.io/deepsky-data/straumheim:latest

Straumheim listening on :8080
  Inputs:  snowplow (/sp), webhook (/webhook), pixel (/px)
  Sinks:   warehouse (postgres), debug (stdout)
  Buffer:  memory (capacity: 10000)
```

### Step 2: Collect

Point your existing trackers at Straumheim. No SDK changes needed.

```javascript
// Snowplow tracker — just change the collector URL
snowplow('newTracker', 'sp', 'collect.yourdomain.com', {
  appId: 'my-app',
  platform: 'web'
});
```

```bash
# Webhook — POST any JSON
curl -X POST https://collect.yourdomain.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"event": "signup", "user_id": "abc123"}'
```

```html
<!-- Pixel — track email opens -->
<img src="https://collect.yourdomain.com/px?event=email_open&campaign=welcome" />
```

### Step 3: Normalize

Every event — regardless of protocol — becomes a Record with a consistent structure:

```json
{
  "id": "01912345-6789-7abc-...",
  "timestamp": "2026-03-22T10:15:30Z",
  "protocol": "snowplow",
  "source": "my-app",
  "payload": { "event": "page_view", "page_url": "..." },
  "flattened": { "event": "page_view", "page_url": "..." },
  "ip": "203.0.113.42",
  "user_agent": "Mozilla/5.0 ..."
}
```

### Step 4: Deliver

Records flow to every configured destination. Postgres gets COPY-batched inserts. Files get JSONL lines. S3 gets rotated objects. Each sink runs independently — if Postgres is slow, the file sink still writes.

---

## Architecture

```
COLLECT (HTTP, protocol-aware)
  Snowplow (/sp)  ·  Webhook (/webhook)  ·  Pixel (/px)
                          │
                          ▼
RECORD (normalize into common structure)
  UUID · timestamps · flatten JSON · infer types · opt-in schema validate
                          │
                          ▼
BUFFER (swappable via interface)
  Go channels (default)  │  SQLite WAL  │  NATS / Redis / Kafka
                          │
                          ▼
SINK (fan-out to destinations)
  Postgres (COPY)  ·  S3/MinIO  ·  ClickHouse  ·  File/Stdout
```

### Key Architectural Decisions

| Decision | Choice | Why |
|----------|--------|-----|
| **Language** | Go | Single binary, great concurrency, no runtime dependencies |
| **HTTP framework** | Chi | Tiny dependency, good middleware, idiomatic |
| **Default buffer** | Go channels | Zero dependencies, sufficient for most workloads |
| **Postgres ingestion** | COPY FROM STDIN | 10-100x faster than INSERT for batch loads |
| **Schema validation** | Opt-in | Don't penalize simple setups with mandatory schema management |
| **Invalid events** | Tag, don't drop | Invalid records go to a separate stream for debugging — never silently lost |
| **File format** | JSONL (v1), Parquet (v2) | JSONL is simple and universal; Parquet deferred to avoid heavy dependencies |
| **Auto-schema** | On by default | New event fields auto-create Postgres columns — no DDL management needed |

---

## Target Users

### Primary: Solo Developers and Small Teams

**The "I just need my events" user.** They're building a product, want event data for analytics, and refuse to pay Segment pricing or manage Kafka. They care about:
- Single Docker container deployment
- Works with existing Snowplow/GTM tracker setup
- Events land in Postgres where they can query them
- Zero ongoing maintenance

### Secondary: Data Engineers at Growing Startups

**The "we outgrew the vendor" user.** They hit Segment's pricing wall or Snowplow's complexity ceiling. They care about:
- Multiple destination support (warehouse + object storage + analytics DB)
- Schema validation as data quality improves
- Durable buffering for reliability
- Clear upgrade path as scale increases

### Expansion: Teams Running the -heim Stack

| Audience | Use Case |
|----------|----------|
| **Analytics engineers** | Raw events in the warehouse, query with SQL or dbt |
| **Product teams** | Instrument everything without cost anxiety |
| **Data teams** | Straumheim collects, Fyrnheim transforms — complete pipeline |
| **Infrastructure teams** | One binary to operate, standard Prometheus metrics |

---

## Business Model: Open Source

### Fully Open Source (MIT)

Everything ships as open source:
- All input protocols (Snowplow, webhook, pixel)
- All sinks (Postgres, S3, ClickHouse, file, stdout)
- All buffer implementations
- Schema validation and registry
- Docker images

### No Commercial Layer (For Now)

Straumheim is infrastructure. It's a tool, not a platform. The value is in running it — not in a hosted version of it. If a commercial layer ever makes sense, it would be:

| Offering | Possible Value |
|----------|---------------|
| **Managed hosting** | Zero-ops event collection for teams that don't want to run Docker |
| **Schema registry** | Hosted schema management with versioning and compatibility checks |
| **Dashboard** | Event volume monitoring, destination health, pipeline metrics |

But the priority is making the open source tool excellent. Commercial comes later, if ever.

---

## Key Product Decisions

### 1. Single Binary, Zero Infrastructure

The default setup requires nothing beyond the binary and a config file. No Kafka. No Redis. No ZooKeeper. Go channels handle buffering. Postgres handles storage.

**Rationale:** The biggest barrier to self-hosted event collection is operational complexity. Eliminate it at the architecture level.

### 2. Snowplow Protocol Compatibility

Straumheim speaks the Snowplow tracker protocol natively. Existing Snowplow tracker SDKs (JavaScript, iOS, Android, GTM) work without modification — just change the collector URL.

**Rationale:** Snowplow has the largest open-source tracker ecosystem. Compatibility means instant access to battle-tested client libraries without building our own SDKs.

### 3. Tag Invalid Records, Don't Drop Them

Events that fail schema validation are tagged (`is_valid: false`) and still delivered to a separate table or stream. Nothing is silently lost.

**Rationale:** Dropping events in a pipeline is the #1 cause of data trust issues. Debug by querying your invalid events — don't discover gaps weeks later.

### 4. Auto-Schema in Postgres

When a new field appears in an event, Straumheim auto-creates the Postgres column. No DDL migration needed.

**Rationale:** Schema management is the second biggest friction point after deployment complexity. Auto-schema makes "just send events" actually work.

### 5. YAML Config, Not Code

The entire pipeline is configured via a single YAML file with environment variable substitution. No plugin system, no scripting language, no DSL.

**Rationale:** Config-as-code is powerful but premature for v1. A YAML file is readable, diffable, and deployable via any config management tool.

### 6. Go, Built for AI-Assisted Development

Go is the implementation language. Clear interfaces, small files, explicit error handling — optimized for building with Claude Code.

**Rationale:** This is a solo-founder project built with maximum AI leverage. Go's simplicity and explicitness make it ideal for AI-assisted development.

---

## Competitive Positioning

### The Gap We Fill

```
"I need raw events in my warehouse"          ← STRAUMHEIM
  (collect, normalize, deliver)
─────────────────────────────────────────────
"I need to transform and model the data"     ← dbt / Fyrnheim
─────────────────────────────────────────────
"I need to query and visualize"              ← Metabase / Grafana
─────────────────────────────────────────────
"I need product analytics"                   ← PostHog / Amplitude
```

Every layer above assumes you have events flowing into your warehouse. Straumheim makes that happen.

### Why Straumheim Wins

| vs. | Their Limitation | Our Advantage |
|-----|-----------------|---------------|
| **Segment** | Per-event pricing. $120/mo for 10K MTU. | Free. Self-hosted. No per-event cost. |
| **Snowplow OSS** | Requires Kafka/Kinesis/PubSub + Spark/Beam. | Single binary. Go channels. Docker run. |
| **Jitsu** | Requires Kafka + Redis. TypeScript monorepo. | Single binary. Zero infrastructure deps. |
| **RudderStack** | Complex self-hosting. Enterprise-focused. | Minimal. Solo dev to small team focused. |
| **PostHog** | Full analytics platform. Heavy. Opinionated. | Pipeline only. Bring your own analytics. |
| **Custom scripts** | Fragile. No batching. No schema. No retry. | Production-grade. COPY batching. Schema validation. |

### Why Self-Hosted Specifically

1. **Cost predictability.** A $5/mo VPS runs Straumheim for unlimited events. No pricing tiers, no overages, no negotiations.
2. **Data sovereignty.** Events never leave your infrastructure. No third-party processing, no vendor DPA needed.
3. **Latency.** Events go directly from collector to warehouse. No cross-region hops through a vendor's cloud.
4. **Reliability.** Your uptime depends on your infrastructure, not a vendor's status page.

---

## Success Metrics

### For Users

- *"I set it up in 10 minutes and my events are in Postgres"*
- *"I switched from Segment and my bill went from $200/mo to $5/mo"*
- *"I didn't have to change my tracker setup — just pointed it at Straumheim"*
- *"It's been running for 6 months and I haven't touched it"*

### For the Project

| Metric | Phase 1 | Phase 2 | Phase 3 |
|--------|---------|---------|---------|
| GitHub stars | 200 | 1,000 | 3,000 |
| Docker pulls | 500 | 5,000 | 50,000 |
| Contributors | 3 | 10 | 30 |
| Sink implementations | 4 | 8 | 15 |

---

## Summary

| Element | Description |
|---------|-------------|
| **The Problem** | Event collection is either expensive (Segment), complex (Snowplow OSS), or fragile (custom scripts) |
| **Why It Persists** | Multi-protocol parsing, batch delivery, schema evolution, and buffering are each solved problems — but wiring them together is a month of work |
| **The Insight** | A single Go binary with a YAML config can replace the entire vendor stack for 90% of products |
| **The Mechanism** | Collect (Snowplow/webhook/pixel) → Normalize (common Record) → Buffer (channels) → Deliver (Postgres/S3/files) |
| **The Distribution** | Open source: Docker container, single binary, YAML config |
| **Phase 1** | Core pipeline: 3 inputs, memory buffer, Postgres + file + stdout sinks |
| **Phase 2** | Destination ecosystem: S3, ClickHouse, BigQuery, Parquet |
| **Phase 3** | Reliability: durable buffer, schema validation, horizontal scale |

---

> **Straumheim: Self-hosted event collection — single binary, zero dependencies, your data on your infrastructure.**

---

*Version 1.0*
*March 2026*
*deepskydata*
