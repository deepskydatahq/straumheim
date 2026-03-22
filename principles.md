# Straumheim — Principles

> These principles govern every design decision, code change, and architecture choice in the Straumheim project. When in doubt, refer back here. When two principles conflict, the one listed earlier in its section generally wins.

## 1. Project philosophy

### 1.1 Simple until proven otherwise

Do not build for scale you don't have. The default deployment is a single Docker container on a €5/month VPS collecting events from a small website. Every feature must work in that context first. Horizontal scaling, distributed queues, and cloud-native orchestration are upgrade paths — not prerequisites.

### 1.2 One binary, zero mandatory dependencies

Straumheim ships as a single statically-linked Go binary. The only thing it needs to run is a config file. Postgres, S3, Redis, NATS — these are destinations and optional buffers, not boot dependencies. If the binary can't start without an external service, something is wrong.

### 1.3 Opt-in complexity

Every feature that adds operational complexity must be off by default. Schema validation, external buffers, TLS configuration, cookie tracking — all opt-in via config. A first-time user should be able to `docker run straumheim` with a minimal config and see events flowing to stdout within 60 seconds.

### 1.4 Boring technology

Prefer well-understood, battle-tested patterns over novel approaches. A Go channel buffer is boring. A Postgres COPY batch insert is boring. Boring is good — it means predictable failure modes, well-documented edge cases, and easy debugging at 2am.

### 1.5 Own your data, own your infra

Straumheim exists because vendor-hosted event pipelines are a black box. Every design choice should reinforce the user's ability to inspect, debug, and control their own data pipeline. No telemetry phone-home. No external service calls. No "cloud-only" features.


## 2. Go principles

### 2.1 Standard library first

Use the Go standard library unless there's a compelling reason not to. `net/http`, `encoding/json`, `database/sql`, `log/slog`, `context`, `sync` — these cover 80% of what Straumheim needs. Chi is the one accepted HTTP dependency because its middleware ecosystem (logging, recovery, CORS) saves significant boilerplate.

### 2.2 Explicit over implicit

No magic. No init() functions that register things globally. No struct tags that drive runtime behavior in surprising ways. If a component needs to be initialized, there's a constructor function. If a dependency is needed, it's passed as a parameter. The wiring happens in `main.go` where you can read the entire startup sequence top to bottom.

### 2.3 Interfaces are discovered, not designed upfront

Define interfaces at the point of consumption, not at the point of implementation. The `Sink` interface exists because the pipeline needs to write to multiple destinations — not because we wanted an abstract "output" concept. Keep interfaces small: 3-5 methods maximum. If an interface has more than 5 methods, it's doing too much.

### 2.4 Errors are values, not exceptions

Every function that can fail returns an error. Errors are wrapped with context using `fmt.Errorf("operation: %w", err)` so you can trace the call path. Never swallow errors silently. Never panic except in truly unrecoverable situations (and even then, prefer logging and graceful shutdown).

```go
// Good
records, err := input.Parse(r)
if err != nil {
    return fmt.Errorf("parsing %s request: %w", input.Protocol(), err)
}

// Bad
records, _ := input.Parse(r)
```

### 2.5 Context flows everywhere

Every function that does I/O or could be cancelled takes `context.Context` as its first parameter. This is how graceful shutdown propagates through the system — when the server stops, contexts get cancelled, buffers flush, sinks close.

### 2.6 Concurrency via goroutines and channels, not shared state

The buffer layer is the concurrency boundary. Inputs write to the buffer from HTTP handler goroutines. A separate goroutine consumes from the buffer and writes to sinks. Communication happens through channels, not shared memory with mutexes. When a mutex is unavoidable (e.g., metrics counters), use `sync.Mutex` or `sync/atomic` — never `sync.RWMutex` unless profiling proves read contention.

### 2.7 Structured logging with slog

Use `log/slog` for all logging. Every log line includes structured fields that make grep/filtering possible. Log levels: DEBUG for internal state, INFO for operational events (startup, shutdown, flush), WARN for recoverable problems (invalid event, destination timeout), ERROR for things that need human attention.

```go
slog.Info("flush completed",
    "sink", sink.Name(),
    "records", count,
    "duration_ms", elapsed.Milliseconds(),
)
```

### 2.8 Small files, clear names

No file over 500 lines. If a file grows beyond that, it's doing too much — split it. File names describe what's inside: `postgres.go` contains the Postgres sink, `flatten.go` contains JSON flattening logic. Package names are short, lowercase, and singular: `record`, `sink`, `buffer`, `input`, `config`.

### 2.9 Tests are documentation

Tests show how a component is meant to be used. Table-driven tests for input parsing, sink writing, and record building. Integration tests for Postgres (using testcontainers or a test DSN). No mocks unless the real thing is genuinely impractical — prefer fakes (in-memory sink, channel-based buffer) that behave like the real implementation.

### 2.10 No premature abstraction

If there's only one implementation of something, don't create an interface for it. The interface emerges when the second implementation arrives. A function is fine. A concrete struct is fine. Indirection has a cost — every layer of abstraction is a layer someone has to understand when debugging.


## 3. Event streaming principles

### 3.1 At-least-once delivery

Straumheim guarantees at-least-once delivery to sinks. Events may be delivered more than once (e.g., after a crash during a batch flush). Sinks that need exactly-once semantics handle deduplication themselves (Postgres: `ON CONFLICT DO NOTHING`, ClickHouse: ReplacingMergeTree). The pipeline never silently drops events.

### 3.2 The buffer is the contract boundary

Everything before the buffer (inputs, record building) is synchronous with the HTTP request. The response goes back to the client once the record is in the buffer. Everything after the buffer (sink writing) is asynchronous. This is the fundamental reliability guarantee: if the client got a 200, the event is buffered.

### 3.3 Backpressure over data loss

If a sink is down or slow, the buffer fills up. When the buffer is full, the HTTP endpoint returns 503 (Service Unavailable) to the client. The client's tracker SDK retries. This is correct behavior — it's better to push back on the client than to silently drop events. The retry is the client SDK's responsibility; Straumheim's responsibility is to never pretend it accepted something it couldn't store.

### 3.4 Batching is a performance optimization, not a feature

Batching exists because databases perform better with bulk inserts than row-by-row writes. The batch size and flush interval are tuning knobs, not semantic guarantees. A batch of 1 must work. A batch of 10,000 must work. The sink interface treats `Write([]Record)` as a batch — the size is determined by the buffer's flush logic.

### 3.5 Order is best-effort, not guaranteed

Within a single HTTP request, events maintain their order. Across requests, order is best-effort. The `Record.Timestamp` (collector time) and `Record.DeviceTime` (client time) exist so that downstream consumers can sort by time. The pipeline does not guarantee global ordering — that's a distributed systems problem that doesn't belong in a single-binary event collector.

### 3.6 Invalid events are data, not errors

An event that fails schema validation is NOT dropped. It's tagged (`is_valid: false`) and delivered to a separate table or file (e.g., `events_invalid` in Postgres, a separate JSONL file). Invalid events are the most valuable debugging signal you have — they tell you what's broken in your tracking implementation. Dropping them is losing information.

### 3.7 Idempotent sinks

Every sink must handle being called with the same record twice. UUIDv7 IDs make this straightforward — Postgres uses `ON CONFLICT (id) DO NOTHING`, file sinks accept duplicates (dedup happens downstream in the warehouse). This principle enables safe retries after partial failures.

### 3.8 Fan-out is simultaneous, not sequential

When multiple sinks are configured, records are written to all of them. A failure in one sink does not block or affect the others. Each sink has its own error handling, retry logic, and backpressure. The pipeline treats sinks as independent consumers.


## 4. Event design principles

### 4.1 Events are immutable facts

A record represents something that happened at a point in time. It is never updated, only appended. If a correction is needed, a new event is emitted. This is the foundational principle of event sourcing and it makes the pipeline dramatically simpler — there are no UPDATE operations, ever.

### 4.2 The collector adds metadata, not meaning

The collector enriches events with metadata it can observe (IP address, User-Agent, referer, collector timestamp) but never interprets or transforms the event payload. IP-to-geo resolution, User-Agent parsing, session stitching — these belong downstream in the data warehouse, not in the collector. The collector's job is to capture the raw signal as faithfully as possible.

### 4.3 Timestamps are the hardest problem

Every record has two timestamps: `timestamp` (when the collector received it) and `device_time` (when the client says it happened). They will disagree. The client's clock may be wrong. The network may have added latency. Batch sends from mobile SDKs may arrive minutes after the events occurred. Always store both. Never trust only one. Use collector timestamp for pipeline ordering; use device timestamp for analytics.

### 4.4 Schema is optional but encouraged

For a personal blog, schemaless events are fine — just send whatever JSON you want. For a SaaS product with a tracking plan, schema validation catches broken tracking before it reaches the warehouse. Straumheim supports both modes and doesn't judge. The config toggle is per-input, not global, so you can validate your own SDK events while accepting arbitrary webhooks.

### 4.5 Flat is better than nested for warehouse destinations

JSON nesting (`{user: {id: 1, name: "timo"}}`) is natural for APIs but painful for SQL queries. The flattening step (`user_id: 1, user_name: "timo"`) happens in the record builder so that warehouse sinks get flat columns. The original nested payload is preserved in the record for non-warehouse sinks. Flattening uses underscore separation with configurable depth limits to prevent explosion from deeply nested objects.

### 4.6 Every event gets a globally unique ID

UUIDv7: time-sortable (the first 48 bits are a millisecond timestamp), globally unique (no coordination), and efficient as a database primary key (monotonically increasing, good for B-tree indexes). Every record gets a UUIDv7 assigned at the collector, regardless of whether the client sent its own ID. The client ID is preserved as a separate field.

### 4.7 Protocol normalization preserves provenance

When a Snowplow event is normalized into a Record, the `protocol` field says "snowplow". When the same event shape arrives as a webhook, it says "webhook". The Record struct is protocol-agnostic, but you can always trace back to how the event entered the system. Protocol-specific fields (Snowplow's `app_id`, `platform`, `dvce_type`) are preserved in the payload, not thrown away.

### 4.8 Design for the 90% case

Most events are page views, button clicks, form submissions, and custom business events. The record structure, flattening logic, and sink implementations are optimized for these. Edge cases (10MB webhook payloads, events with 500 nested levels, binary payloads) are handled gracefully (size limits, depth limits, rejection with clear errors) but don't drive the architecture.


## 5. Infrastructure and operations principles

### 5.1 Health checks are non-negotiable

`GET /health` returns 200 when the server is ready to accept events, 503 when it's not (buffer full, shutting down). This is the minimum viable observability. Docker, Kubernetes, and any load balancer use this to route traffic correctly.

### 5.2 Graceful shutdown

When Straumheim receives SIGTERM: (1) stop accepting new HTTP requests, (2) wait for in-flight requests to complete (with a timeout), (3) flush the buffer to sinks, (4) close all sinks, (5) exit. Data in the buffer at shutdown time must not be lost. The in-memory buffer has an inherent risk here — this is why SQLite WAL exists as an upgrade path.

### 5.3 Configuration is static at boot

Straumheim reads its config file once at startup. There is no hot-reload, no config API, no runtime reconfiguration. If you change the config, you restart the process. This is dramatically simpler to reason about and debug. Docker and Kubernetes make restarts cheap.

### 5.4 Metrics are Prometheus-native

Expose `/metrics` in Prometheus exposition format. Key metrics: events received (by input, by protocol), events buffered, events flushed (by sink), flush duration, flush errors, buffer utilization, HTTP response codes. These metrics tell you everything about pipeline health without looking at logs.

### 5.5 Docker-first, but not Docker-only

The primary distribution is a Docker image built from scratch (just the binary + ca-certificates). But the binary also runs perfectly fine as a systemd service, a lambda function, or a raw process. No Docker-specific assumptions in the code.

### 5.6 Configuration via YAML + env vars

The config file is YAML with `${ENV_VAR}` substitution. Secrets (database passwords, API keys) come from environment variables, never from the config file. The config file can be committed to git; the secrets cannot.

### 5.7 Fail loud, recover quiet

When something goes wrong (sink connection lost, invalid config, schema file not found), log an ERROR with full context. When it recovers (sink reconnected, retry succeeded), log an INFO. Never log the same error repeatedly in a tight loop — use exponential backoff or error aggregation.

### 5.8 No external state

Straumheim stores no state outside of what's in its buffer and what it writes to sinks. There's no internal database, no state file, no coordination service. If you lose the process, you lose the in-memory buffer (upgrade to SQLite WAL if this matters). Everything else can be reconstructed by restarting with the same config.


## 6. Code organization principles

### 6.1 Internal packages for implementation

Everything under `internal/` is private to the Straumheim binary. This is a Go convention that the compiler enforces — nothing outside the module can import these packages. The public API is the HTTP endpoints and the config file format, not Go types.

### 6.2 One concern per package

`record/` knows about records. `sink/` knows about destinations. `buffer/` knows about queuing. `input/` knows about HTTP protocols. They communicate through interfaces defined at the consumer, not the provider. The `pipeline/` package is the only place that knows about all of them — it's the orchestrator.

### 6.3 Dependencies point inward

`record` depends on nothing (it's the core domain). `buffer` depends on `record`. `sink` depends on `record`. `input` depends on `record`. `pipeline` depends on everything. `config` is standalone. No circular dependencies, ever.

```
config ─────────────────────────┐
                                │
record ◄── input                │
   ▲        ▲                   │
   │        │                   ▼
   ├── buffer ◄── pipeline ◄── main
   │                  │
   └── sink ◄─────────┘
```

### 6.4 main.go is the wiring

`cmd/straumheim/main.go` reads config, constructs all components, wires them together, starts the HTTP server, and handles shutdown signals. It's the only file that imports everything. It should be readable as a narrative: "parse config, build inputs, build buffer, build sinks, build pipeline, start server, wait for signal, shutdown."

### 6.5 No global variables

No package-level `var` that holds runtime state. No singleton patterns. No global logger (pass `*slog.Logger` as a dependency). The only acceptable package-level variables are constants and the arrow markers for tests (`var _ Sink = (*PostgresSink)(nil)`).

### 6.6 Constructors return concrete types

`NewPostgresSink(cfg SinkConfig) (*PostgresSink, error)` — not `Sink`. The interface is used at the call site where polymorphism is needed (the pipeline stores `[]Sink`). The constructor returns the concrete type so tests can access implementation-specific methods.


## 7. Security principles

### 7.1 Accept anything, trust nothing

The collector accepts events from the public internet. Every input is untrusted. JSON payloads have size limits (configurable, default 1MB). URL path lengths are bounded. Header sizes are bounded. No input can cause the collector to allocate unbounded memory.

### 7.2 No authentication by default

The collector is an event ingestion endpoint — it's meant to receive data from client-side JavaScript. Adding authentication to the collector itself is usually wrong (the auth token would be visible in the browser). Instead, use a reverse proxy (nginx, Caddy, Cloudflare) for rate limiting, IP filtering, and CORS. Straumheim sets sensible CORS defaults but doesn't try to be a security layer.

### 7.3 Write keys are identifiers, not secrets

If a `write_key` is configured per input source, it's used for routing and attribution (which source sent this event), not for security. It's equivalent to Segment's write key — visible in client-side code, useful for separating environments (dev/staging/prod), not a security boundary.

### 7.4 Secrets never appear in logs

Database connection strings, API keys, and credentials are redacted in log output and error messages. The config loader masks `${ENV_VAR}` values in diagnostic output. Stack traces and error messages never include raw credentials.

### 7.5 Payload data is opaque

Straumheim does not inspect, filter, or redact PII in event payloads. It is a transport layer. PII handling (anonymization, pseudonymization, deletion) belongs in the warehouse layer where you have the full context of your data governance requirements. Straumheim stores what it receives, faithfully.


## 8. Naming principles

### 8.1 Domain language

Use consistent terminology throughout code, config, docs, and logs:

| Term | Meaning | Not |
|------|---------|-----|
| Record | Normalized event wrapper | Envelope, message, event (too overloaded) |
| Input | Protocol-specific HTTP handler | Source, collector, receiver |
| Buffer | In-process queue between input and sink | Queue, broker, stream |
| Sink | Output destination | Destination, target, output |
| Flush | Write accumulated records from buffer to sink | Drain, emit, push |
| Protocol | Wire format (snowplow, webhook, pixel) | Type, kind, format |
| Payload | The actual event data inside a record | Body, data, content |
| Schema | JSON Schema definition for validation | Contract, spec, type |

### 8.2 Go naming conventions

- Package names: short, singular, lowercase (`record`, `sink`, `buffer`)
- Interface names: verbed nouns or `-er` suffix (`Sink`, `Buffer`, `Flusher`)
- Constructor functions: `New{Type}(...)` (`NewPostgresSink`, `NewMemoryBuffer`)
- Config structs: `{Component}Config` (`SinkConfig`, `BufferConfig`)
- Error variables: `Err{Description}` (`ErrBufferFull`, `ErrSinkClosed`)
- Test files: `{file}_test.go` in the same package (white-box testing)
- No stuttering: `sink.PostgresSink`, not `sink.SinkPostgres`

### 8.3 Config key naming

YAML keys use `snake_case`. Environment variables use `SCREAMING_SNAKE_CASE`. Mapping is direct: `server.listen_port` → `STRAUMHEIM_SERVER_LISTEN_PORT`. Nesting in YAML maps to underscore separation in env vars.


## 9. Documentation principles

### 9.1 README is the landing page

The README answers three questions: what is this, how do I run it, where do I learn more. It includes a working `docker run` command, a minimal config file, and a curl command that sends a test event. Someone should go from zero to events-flowing in under 5 minutes.

### 9.2 Config file is self-documenting

`config.example.yaml` includes comments explaining every option, its default value, and when you'd change it. The example config is the primary reference for operators — it's more useful than a docs site for a tool this size.

### 9.3 Code comments explain why, not what

`// Flush on interval OR count, whichever comes first` — good. `// Loop over records` — bad. Comments exist for non-obvious decisions, gotchas, and the reasoning behind a trade-off. The code itself should be clear enough to explain what it does.

### 9.4 ADRs for significant decisions

Architecture Decision Records live in `docs/adr/`. One file per decision: the context, the options considered, the decision, and the consequences. "Why did we choose Chi over stdlib?" "Why UUIDv7 instead of UUIDv4?" "Why is schema validation opt-in?" These are the questions future-you will ask.
