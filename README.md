# Straumheim

**Self-hosted event collection. Single binary. Zero dependencies. Your data, your infrastructure.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://go.dev)
[![Docker](https://img.shields.io/badge/ghcr.io-straumheim-blue)](https://ghcr.io/deepskydatahq/straumheim)

---

Straumheim collects events from Snowplow trackers, webhooks, and tracking pixels, normalizes them into a common record, and delivers them to Postgres, ClickHouse, JSONL files, or stdout. One Go binary. One YAML config. No Kafka, no Redis, no vendor lock-in.

```
docker pull ghcr.io/deepskydatahq/straumheim:latest
```

## Quickstart

Clone the repo and start with docker-compose:

```bash
git clone https://github.com/deepskydatahq/straumheim.git
cd straumheim
docker compose up -d
```

This starts Straumheim + Postgres. Send a test event:

```bash
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"event": "signup", "user_id": "abc123", "plan": "pro"}'
```

Query it:

```bash
docker compose exec postgres psql -U straumheim -d events \
  -c "SELECT id, protocol, payload FROM events;"
```

Try the other inputs:

```bash
# Snowplow tracker protocol
curl "http://localhost:8080/sp/i?e=pv&url=https://example.com&aid=my-app"

# Tracking pixel (email opens, AMP pages)
curl "http://localhost:8080/px?event=email_open&campaign=welcome"
```

## Architecture

```
COLLECT (HTTP, protocol-aware)
  Snowplow (/sp)  ·  Webhook (/webhook)  ·  Pixel (/px)
                          │
                          ▼
RECORD (normalize into common structure)
  UUIDv7 · timestamps · flatten JSON · collector metadata
                          │
                          ▼
BUFFER (in-memory, flush by count or interval)
  Go channels  ·  backpressure when full
                          │
                          ▼
SINK (fan-out to all destinations independently)
  Postgres (COPY)  ·  ClickHouse (HTTP)  ·  JSONL files  ·  Stdout
```

Every event becomes a **Record**:

```json
{
  "id": "019d14e2-11b0-77be-8f5f-812eb5a1f89a",
  "timestamp": "2026-03-22T10:15:30Z",
  "protocol": "webhook",
  "source": "my-app",
  "ip": "203.0.113.42",
  "user_agent": "Mozilla/5.0 ...",
  "payload": {"event": "signup", "user_id": "abc123", "plan": "pro"},
  "flattened": {"event": "signup", "user_id": "abc123", "plan": "pro"}
}
```

## Inputs

| Input | Endpoint | Description |
|-------|----------|-------------|
| **Webhook** | `POST /webhook` | Generic JSON. Schema-addressed: `POST /webhook/{vendor}/{name}/{version}` |
| **Snowplow** | `GET /sp/i`, `POST /sp/tp2` | Drop-in replacement for Snowplow collector. Works with all Snowplow SDKs (JS, iOS, Android, GTM). Cookie-based `network_userid`. |
| **Pixel** | `GET /px` | 1x1 transparent GIF. Query params become payload. For emails, AMP, constrained environments. |

## Sinks

| Sink | Type | Description |
|------|------|-------------|
| **Postgres** | `postgres` | `COPY FROM STDIN` batch ingestion. Auto-creates table. Auto-adds columns for new fields. |
| **ClickHouse** | `clickhouse` | HTTP interface with JSONEachRow. MergeTree engine. Auto-schema. Sub-second analytical queries. |
| **File** | `file` | JSONL with time-based rotation. Query with DuckDB, jq, or grep. |
| **Stdout** | `stdout` | JSON lines for debugging and log aggregation. |

## Configuration

Single YAML file. Secrets via `${ENV_VAR}` substitution. See [`config.example.yaml`](config.example.yaml) for the full reference.

```yaml
server:
  host: 0.0.0.0
  port: 8080
  cors:
    allowed_origins: ["*"]    # or ["https://mysite.com"]

inputs:
  webhook:
    enabled: true
  snowplow:
    enabled: true
    snowplow:
      cookie:
        enabled: true
        name: sp
        ttl: 8760h             # 1 year
  pixel:
    enabled: true

buffer:
  type: memory
  capacity: 10000
  flush_interval: 5s
  flush_count: 500

sinks:
  - name: warehouse
    type: postgres
    dsn: ${POSTGRES_DSN}
    table: events
    auto_schema: true

  - name: analytics
    type: clickhouse
    endpoint: https://ch.example.com
    database: events
    table: events
    username: ${CH_USER}
    password: ${CH_PASSWORD}

  - name: archive
    type: file
    output_dir: /var/lib/straumheim/events
    rotation_interval: 5m

  - name: debug
    type: stdout
```

## Observability

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Returns `{"status": "ok"}` when ready to accept events |
| `GET /metrics` | Prometheus text format |

Metrics exposed:

- `straumheim_records_received_total` — by protocol
- `straumheim_records_delivered_total` — by sink
- `straumheim_records_failed_total` — by sink
- `straumheim_buffer_size_current`
- `straumheim_flush_duration_seconds` — by sink

Structured request logging via slog (method, path, status, duration, remote_addr). CORS and panic recovery middleware included.

## Production Deployment

A production-ready docker-compose with Caddy (auto-TLS) is included:

```bash
cp .env.example .env
# Edit .env: set DOMAIN, POSTGRES_PASSWORD, POSTGRES_DSN
docker compose -f docker-compose.prod.yaml up -d
```

This gives you:
- **Caddy** — automatic HTTPS via Let's Encrypt
- **Straumheim** — not exposed directly, proxied through Caddy
- **Postgres** — persistent volume, healthcheck

See [`docker-compose.prod.yaml`](docker-compose.prod.yaml) and [`deploy/Caddyfile`](deploy/Caddyfile).

## Why Straumheim

| vs. | Their limitation | Straumheim |
|-----|-----------------|------------|
| **Segment** | $120/mo for 10K MTU. Per-event pricing. | Free. Self-hosted. Unlimited events. |
| **Snowplow OSS** | Requires Kafka/Kinesis + Spark/Beam. | Single binary. `docker run`. |
| **Jitsu** | Requires Kafka + Redis. | Zero infrastructure dependencies. |
| **Custom scripts** | Fragile. No batching. No auto-schema. | COPY batching. Auto-schema. Fan-out. |

## Roadmap

See [VALUES.md](VALUES.md) for the complete value ladder.

**Shipped:**
- Multi-protocol collection (Snowplow, webhook, pixel)
- Record normalization (UUIDv7, JSON flattening, metadata)
- Batch delivery with fan-out
- Postgres sink (COPY + auto-schema)
- ClickHouse sink (HTTP + auto-schema)
- JSONL file sink with rotation
- Prometheus metrics
- CORS, request logging, panic recovery
- Production deployment with Caddy auto-TLS

**Next:**
- SQLite WAL durable buffer
- S3 object storage sink
- Schema validation (JSON Schema, opt-in)

## License

MIT — see [LICENSE](LICENSE).

---

*Part of the `-heim` family alongside [Fyrnheim](https://github.com/deepskydatahq/fyrnheim) (data transformation).*
