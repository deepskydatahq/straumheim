# Straumheim — Project brief

> Lightweight, self-hosted event data pipeline. Collect events from any source, normalize them into records, buffer for reliability, and deliver to one or more destinations. Ships as a single Go binary in a Docker container.

Part of the `-heim` family alongside [Fyrnheim](https://github.com/...) (data transformation).

## Design principles

1. **Single binary, minimal dependencies.** `docker run straumheim` with a config file gets you a production-ready event collector. No Kafka, no Redis, no external queue required for the default setup.
2. **Schema validation is opt-in.** Small websites don't need it. When you do want it, JSON Schema files live on the local filesystem (or S3) and get validated in microseconds.
3. **Destination flexibility through a clean interface.** Start with Postgres + file output. Adding ClickHouse, BigQuery, or Snowflake later means implementing one interface — no changes to the collector or buffer.
4. **Buffer is swappable.** Default is in-memory Go channels (zero deps). Upgrade path: SQLite WAL for single-node durability, NATS/Redis/Kafka for horizontal scale.
5. **AI-agent friendly.** Go is the implementation language. Clear interfaces, small files, explicit error handling — optimized for building with Claude Code.

## Prior art and what we take from each

### Buz (silverton-io/buz)

- **Keep:** Record wrapping pattern (UUID, timestamps, protocol tag, schema ref, validation status, payload), multi-protocol input, fan-out to multiple destinations, single-binary deployment, local filesystem schema registry
- **Skip:** 24+ destination implementations (scope creep), Go Gin framework (use stdlib `net/http` or Chi for simplicity), telemetry phone-home

### Jitsu / Bulker (jitsucom/jitsu, jitsucom/bulker)

- **Keep:** JSON flattening for warehouse destinations (`{a: {b: 1}}` → `a_b`), type inference from values, streaming vs batching mode per destination, deduplication by primary key, auto-column creation in databases
- **Skip:** Hard Kafka dependency, Postgres + Redis required infrastructure, Next.js console webapp, TypeScript monorepo complexity

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ COLLECT — HTTP endpoints, protocol-aware                        │
│                                                                 │
│  Snowplow (/sp)    Webhook (/webhook)   Pixel (/px)   + custom │
└───────┬──────────────────┬──────────────────┬───────────────────┘
        │                  │                  │
        ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│ RECORD — Normalize into a common structure                      │
│                                                                 │
│  UUID · timestamps · flatten JSON · infer types                 │
│  protocol tag · vendor/namespace · opt-in schema validate       │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ BUFFER — Swappable via interface                                │
│                                                                 │
│  Go channels (default)  │  SQLite WAL  │  NATS / Redis / Kafka │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ SINK — Destination interface, stream or batch mode per dest     │
│                                                                 │
│  Postgres    S3/MinIO     ClickHouse    File/Stdout    + custom │
│  (COPY)      (Parquet)    (batch)       (debug)                 │
└─────────────────────────────────────────────────────────────────┘
```

## The record

Every event, regardless of input protocol, gets normalized into a `Record`. This is the core data structure that flows through the entire system.

```go
type Record struct {
    ID            string            `json:"id"`             // UUIDv7
    Timestamp     time.Time         `json:"timestamp"`      // Collector receive time
    DeviceTime    *time.Time        `json:"device_time"`    // Client-reported time (if present)
    Protocol      string            `json:"protocol"`       // "snowplow", "webhook", "pixel"
    Source        string            `json:"source"`         // App ID or source identifier

    // Schema (opt-in)
    Schema        string            `json:"schema"`         // e.g. "com.example/pageview/v1.0"
    Vendor        string            `json:"vendor"`         // e.g. "com.example"
    SchemaVersion string            `json:"schema_version"` // e.g. "1.0"
    IsValid       *bool             `json:"is_valid"`       // nil = not validated, true/false = result

    // Payload
    Payload       map[string]any    `json:"payload"`        // The actual event data
    Flattened     map[string]any    `json:"flattened"`      // Payload after JSON flattening

    // Collector metadata
    IP            string            `json:"ip"`
    UserAgent     string            `json:"user_agent"`
    Referer       string            `json:"referer"`

    // Internal
    ReceivedAt    time.Time         `json:"-"`              // Internal processing time
}
```

**Key decisions:**
- UUIDv7 for IDs — time-sortable, no coordination needed
- `Flattened` is populated lazily when a warehouse sink needs it
- `IsValid` is a pointer: `nil` means schema validation was skipped (opt-in), `true`/`false` means it ran
- Invalid records are NOT dropped — they're tagged and still delivered (separate table or stream for debugging)

## Core interfaces

### Input (collector endpoint)

```go
type Input interface {
    // Register routes on the HTTP mux
    Register(mux *http.ServeMux, pipeline Pipeline) error
    // Protocol name for record tagging
    Protocol() string
}
```

Each input gets its own route prefix and knows how to parse its wire format into `[]Record`.

### Buffer

```go
type Buffer interface {
    Push(ctx context.Context, records []Record) error
    Consume(ctx context.Context, handler func([]Record) error) error
    Close() error
}
```

Default implementation: Go channel with configurable capacity. Consume runs in a goroutine, batches records by count or time interval, and calls the handler.

### Sink (destination)

```go
type Sink interface {
    Init(cfg SinkConfig) error
    Write(ctx context.Context, records []Record) error
    Flush(ctx context.Context) error
    Close() error
    Mode() SinkMode  // SinkModeStream or SinkModeBatch
}

type SinkMode int
const (
    SinkModeStream SinkMode = iota  // Write immediately per record/small batch
    SinkModeBatch                    // Accumulate, flush on interval or threshold
)
```

**Postgres sink specifics:**
- Uses `COPY FROM STDIN` for batch mode (fastest Postgres ingestion)
- Auto-creates the events table on first write
- Auto-adds columns when new fields appear in records (Jitsu-style)
- Deduplication via `ON CONFLICT (id) DO NOTHING`

**File sink specifics:**
- JSONL for v1 (one JSON object per line)
- Parquet as fast-follow (better for DuckDB / analytics queries)
- Writes to local fs or S3-compatible storage (MinIO)
- Rotates files by time interval or size

## Config

Single YAML file with environment variable overrides (`${VAR}` syntax).

```yaml
server:
  host: 0.0.0.0
  port: 8080

inputs:
  snowplow:
    enabled: true
    path: /sp
    cookie:
      enabled: true
      name: sp
      domain: ""          # auto-detect from request host
      ttl: 8760h          # 1 year
  webhook:
    enabled: true
    path: /webhook
  pixel:
    enabled: true
    path: /px

buffer:
  type: memory          # memory | sqlite | nats
  capacity: 10000       # for memory buffer
  flush_interval: 5s
  flush_count: 500

sinks:
  - name: warehouse
    type: postgres
    mode: batch
    dsn: ${POSTGRES_DSN}
    table: events
    batch_size: 1000
    flush_interval: 10s
    auto_schema: true    # auto-create columns

  - name: debug
    type: stdout
    mode: stream

  - name: archive
    type: s3
    mode: batch
    bucket: ${S3_BUCKET}
    prefix: events/
    format: jsonl        # jsonl | parquet
    rotation: 5m

schema:
  enabled: false          # opt-in
  backend: filesystem     # filesystem | s3
  path: ./schemas
  cache_ttl: 300s
```

## Input protocol details

### Snowplow (/sp)

Compatible with Snowplow tracker SDKs (JS tracker, GTM tag, iOS/Android SDKs).

- `GET /sp/i` — pixel endpoint (query string encoded)
- `POST /sp/tp2` — tracker protocol v2 (JSON body, base64 payload)
- `POST /sp/com.snowplowanalytics.snowplow/tp2` — full path variant
- Sets standard `Set-Cookie` for network_userid (sp domain cookie)
- Returns 1x1 transparent GIF for GET, 200 OK for POST

### Webhook (/webhook)

Generic JSON POST endpoint.

- `POST /webhook` — arbitrary JSON, no schema requirement
- `POST /webhook/{vendor}/{name}/{version}` — schema-addressed webhook
- Content-Type must be application/json
- Returns the record ID in the response for traceability

### Pixel (/px)

Lightweight tracking for emails, constrained environments.

- `GET /px` — query params become the payload
- `GET /px/{vendor}/{name}/{version}` — schema-addressed pixel
- Returns 1x1 transparent GIF
- Cache headers set to prevent browser caching

## Project structure

```
straumheim/
├── cmd/
│   └── straumheim/
│       └── main.go              # Entrypoint, wires everything together
├── internal/
│   ├── config/
│   │   └── config.go            # YAML parsing, env var expansion
│   ├── record/
│   │   ├── record.go            # Record struct and builders
│   │   ├── flatten.go           # JSON flattening logic
│   │   └── validate.go          # JSON Schema validation (opt-in)
│   ├── input/
│   │   ├── input.go             # Input interface
│   │   ├── snowplow.go          # Snowplow tracker protocol
│   │   ├── webhook.go           # Generic webhook
│   │   └── pixel.go             # Pixel tracking
│   ├── buffer/
│   │   ├── buffer.go            # Buffer interface
│   │   ├── memory.go            # Go channel implementation
│   │   └── sqlite.go            # SQLite WAL implementation (future)
│   ├── sink/
│   │   ├── sink.go              # Sink interface
│   │   ├── postgres.go          # Postgres with COPY batching
│   │   ├── file.go              # JSONL / Parquet file writer
│   │   ├── s3.go                # S3-compatible object storage
│   │   └── stdout.go            # Debug output
│   ├── schema/
│   │   ├── registry.go          # Schema registry interface
│   │   ├── filesystem.go        # Local fs backend
│   │   └── cache.go             # In-memory LRU cache
│   └── pipeline/
│       └── pipeline.go          # Wires input → record → buffer → sink
├── config.example.yaml
├── Dockerfile
├── Makefile
├── go.mod
└── README.md
```

## v1 scope

What ships first:

- [ ] Snowplow input (GET + POST, cookie handling)
- [ ] Webhook input (arbitrary + schema-addressed)
- [ ] Pixel input
- [ ] Record builder with JSON flattening
- [ ] In-memory buffer (Go channels)
- [ ] Postgres sink with COPY batching + auto-schema
- [ ] Stdout sink (debug)
- [ ] JSONL file sink (local fs)
- [ ] YAML config with env var substitution
- [ ] Docker image (scratch base, single binary)
- [ ] Health check endpoint (/health)
- [ ] Metrics endpoint (/metrics, Prometheus format)

What comes next (v2):

- [ ] S3-compatible file sink
- [ ] Parquet output format
- [ ] SQLite WAL buffer
- [ ] ClickHouse sink
- [ ] Schema validation (JSON Schema)
- [ ] Schema registry (filesystem + S3)
- [ ] Segment-compatible input (/v1/track, /v1/identify, /v1/page)
- [ ] BigQuery sink
- [ ] NATS buffer
- [ ] Deduplication by record ID in database sinks

## Deployment

### Minimal (single Docker container)

```bash
docker run -d \
  -p 8080:8080 \
  -v ./config.yaml:/etc/straumheim/config.yaml \
  -e POSTGRES_DSN="postgres://user:pass@host:5432/analytics" \
  ghcr.io/deepsky-data/straumheim:latest
```

### Docker Compose (with Postgres)

```yaml
services:
  straumheim:
    image: ghcr.io/deepsky-data/straumheim:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/etc/straumheim/config.yaml
    environment:
      POSTGRES_DSN: postgres://straumheim:secret@postgres:5432/events
    depends_on:
      - postgres

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: events
      POSTGRES_USER: straumheim
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

## Naming conventions

| Concept | Name | Why |
|---------|------|-----|
| Normalized event wrapper | Record | Simple, generic, no protocol baggage |
| HTTP endpoints | Input | They're inputs to the system |
| Queue/buffer layer | Buffer | What it does |
| Output destinations | Sink | Industry standard term |
| Record building | Builder | Follows Go conventions |
| Config file | config.yaml | Predictable, discoverable |

## Resolved decisions

1. **HTTP framework:** Chi. Tiny dependency, good middleware ecosystem (logging, recovery, CORS), idiomatic Go.
2. **File format:** JSONL for v1. Parquet deferred to v2 — avoids heavy arrow/parquet library dependencies early on.
3. **Auto-schema in Postgres:** Aggressive by default. Columns are auto-created when new fields appear in records. This is the key DX win for small-site setups — no DDL management needed.
4. **Cookie handling for Snowplow:** Supported and configurable. First-party cookie for `network_userid` is enabled by default but can be disabled in config. Cookie domain, name, and TTL are all configurable.

## Open questions

1. **Project repository:** `github.com/deepsky-data/straumheim` or personal GitHub?
