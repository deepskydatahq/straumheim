# Straumheim

**Self-hosted event collection — single binary, zero dependencies, your data on your infrastructure.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev)

---

Straumheim is a lightweight, self-hosted event data pipeline. It collects events from Snowplow trackers, webhooks, and tracking pixels, normalizes them into a common record, and delivers them to Postgres, JSONL files, and stdout — all from a single Go binary configured with a YAML file.

No Kafka. No Redis. No vendor lock-in. Point your trackers at it, configure your destinations, and your events flow to infrastructure you own.

## Quickstart

Create a `docker-compose.yaml`:

```yaml
services:
  straumheim:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/etc/straumheim/config.yaml
    environment:
      POSTGRES_DSN: postgres://straumheim:secret@postgres:5432/events?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: events
      POSTGRES_USER: straumheim
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U straumheim -d events"]
      interval: 2s
      timeout: 5s
      retries: 5
```

Create a `config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080

inputs:
  webhook:
    enabled: true
  snowplow:
    enabled: true
    snowplow:
      cookie:
        enabled: true
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

  - name: debug
    type: stdout
```

Start everything:

```bash
docker compose up -d
```

Send a test event:

```bash
# Webhook
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"event": "signup", "user_id": "abc123", "plan": "pro"}'

# Snowplow (tracker protocol GET)
curl "http://localhost:8080/sp/i?e=pv&url=https://example.com&aid=my-app"

# Pixel (email open tracking)
curl "http://localhost:8080/px?event=email_open&campaign=welcome"
```

Query your events:

```bash
docker compose exec postgres psql -U straumheim -d events -c "SELECT id, protocol, source, payload FROM events;"
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
  Go channels (default)  ·  backpressure on full
                          │
                          ▼
SINK (fan-out to all destinations)
  Postgres (COPY)  ·  JSONL files  ·  Stdout
```

Every event — regardless of input protocol — becomes a **Record** with a consistent structure:

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

## Features

### Inputs

| Input | Endpoint | Description |
|-------|----------|-------------|
| **Webhook** | `POST /webhook` | Generic JSON POST. Schema-addressed variant: `POST /webhook/{vendor}/{name}/{version}` |
| **Snowplow** | `GET /sp/i`, `POST /sp/tp2` | Compatible with all Snowplow tracker SDKs (JS, iOS, Android, GTM). Cookie-based `network_userid`. Just change the collector URL. |
| **Pixel** | `GET /px` | 1x1 transparent GIF. Query params become payload fields. Schema-addressed variant: `GET /px/{vendor}/{name}/{version}` |

### Sinks

| Sink | Type | Description |
|------|------|-------------|
| **Postgres** | `postgres` | `COPY FROM STDIN` batch ingestion. Auto-creates table. Auto-adds columns for new fields. |
| **File** | `file` | JSONL files with time-based rotation. Queryable with DuckDB, jq, or grep. |
| **Stdout** | `stdout` | JSON lines to stdout. Useful for debugging and log aggregation. |

### Observability

- `GET /health` — returns `{"status": "ok"}` when ready
- `GET /metrics` — Prometheus text format with pipeline metrics:
  - `straumheim_records_received_total` (by protocol)
  - `straumheim_records_delivered_total` (by sink)
  - `straumheim_records_failed_total` (by sink)
  - `straumheim_buffer_size_current`
  - `straumheim_flush_duration_seconds` (by sink)

## Configuration

Straumheim is configured with a single YAML file. Environment variables are substituted with `${VAR}` syntax. See [`config.example.yaml`](config.example.yaml) for a fully commented reference.

### Server

```yaml
server:
  host: 0.0.0.0    # bind address (default: 0.0.0.0)
  port: 8080        # listen port (default: 8080)
```

### Inputs

All inputs default to **disabled** except webhook. Enable them in config:

```yaml
inputs:
  webhook:
    enabled: true
    path: /webhook              # route prefix

  snowplow:
    enabled: true
    snowplow:
      cookie:
        enabled: true           # first-party cookie for network_userid
        name: sp                # cookie name (default: sp)
        domain: ""              # empty = browser auto-detects
        ttl: 8760h              # 1 year

  pixel:
    enabled: true
    path: /px
```

### Buffer

```yaml
buffer:
  type: memory        # only option for now (SQLite WAL planned)
  capacity: 10000     # max buffered records before backpressure
  flush_interval: 5s  # flush after this duration
  flush_count: 500    # flush after this many records
```

### Sinks

```yaml
sinks:
  - name: warehouse
    type: postgres
    dsn: ${POSTGRES_DSN}        # connection string via env var — never plaintext
    table: events               # target table (auto-created)
    auto_schema: true           # auto-add columns for new fields

  - name: archive
    type: file
    output_dir: /var/lib/straumheim/events
    rotation_interval: 5m       # new file every 5 minutes

  - name: debug
    type: stdout
```

## Why Straumheim

| vs. | Their limitation | Straumheim |
|-----|-----------------|------------|
| **Segment** | Per-event pricing. $120/mo for 10K MTU. | Free. Self-hosted. No per-event cost. |
| **Snowplow OSS** | Requires Kafka/Kinesis/PubSub + Spark/Beam. | Single binary. Go channels. `docker run`. |
| **Jitsu** | Requires Kafka + Redis. | Single binary. Zero infrastructure deps. |
| **Custom scripts** | Fragile. No batching. No schema. | Production-grade. COPY batching. Auto-schema. |

## Roadmap

See [VALUES.md](VALUES.md) for the complete value ladder. Currently shipped:

- **L01** Multi-protocol collection (Snowplow, webhook, pixel)
- **L02** Record normalization (UUIDv7, flattening, metadata)
- **L03** Batch delivery (memory buffer, fan-out)
- **L04** Warehouse sink (Postgres with COPY + auto-schema)
- **L05** File storage (JSONL with rotation) — S3 and Parquet planned

Next up: SQLite WAL durable buffer, S3 sink, schema validation.

## License

MIT — see [LICENSE](LICENSE).

---

*Part of the `-heim` family alongside [Fyrnheim](https://github.com/deepskydatahq/fyrnheim) (data transformation).*
