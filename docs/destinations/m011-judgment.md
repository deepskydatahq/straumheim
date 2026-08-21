# M011 product judgment

Judgment date: 2026-08-21

Result: **blocked — 13 testing criteria pass, cleanup remains; the two-instance mission outcome also requires Render billing**

| # | Mission criterion | Result | Evidence |
|---:|---|---|---|
| 1 | BigQuerySink satisfies Sink | Pass | Compile-time assertion in `internal/sink/bigquery.go` |
| 2 | Init validates destination/location and creates stable table | Pass | Required-field validation, production dataset location check, 404-only create, and metadata unit assertions |
| 3 | One Storage Write append per batch | Pass | Fake appender test sends two records in one call; production uses one `AppendRows` |
| 4 | Core and JSON values map deterministically | Pass | Dynamic-protobuf test decodes timestamps, nullable bool, strings, payload JSON, and flattened JSON |
| 5 | Empty/error behavior is actionable | Pass | Empty no-op plus serialization, immediate append, and async result error tests |
| 6 | Resources close cleanly | Pass | Stream/client/admin cleanup and joined-error tests |
| 7 | YAML/createSinks integration | Pass | Config and command tests cover all destination identifiers and optional inflight bound |
| 8 | No credentials committed/logged | Pass | Config/examples contain only ADC path and destination placeholders; secret-pattern scan clean |
| 9 | Tests require no live GCP project | Pass | Metadata/writer factories and fakes cover sink behavior |
| 10 | Go quality gates | Pass | gofmt, `go test ./...`, `go test -race ./...`, `go build ./...`, and `go vet ./...`; Docker scratch build passes |
| 11 | HTTPS event matches BigQuery row | Pass | Twelve public Render Free HTTPS IDs matched twelve BigQuery rows and JSON values |
| 12 | Live batch and duplicate semantics | Pass | Twelve-event public batch passed; a controlled ID produced two raw rows, bounded by documented ID deduplication |
| 13 | Proof resources removed | **Blocked** | Free Render service/secrets are deleted; BigQuery `events` and proof key remain for the resumed two-instance proof |
| 14 | Decision and Cloud Storage boundary documented | Pass | `docs/destinations/bigquery.md` selects direct default-stream delivery and reserves GCS for archive/replay |

## Implemented artifacts

- `internal/sink/bigquery.go`
- `internal/sink/bigquery_test.go`
- BigQuery config and `createSinks` integration/tests
- BigQuery dependency pinned in `go.mod`/`go.sum`
- `config.example.yaml`
- `render.yaml` ADC/destination placeholders
- `deploy/render/config.bigquery.example.yaml`
- `docs/destinations/bigquery.md`
- `docs/destinations/bigquery-proof.md`

## Unblock condition

1. Add Render payment information for the two Starter instances.
2. Re-run the already prepared service-create command with the supplied secret files.
3. Capture two-instance healthy-peer evidence; public HTTPS, batch, JSON, duplicate, and shutdown behavior already pass on Free.
4. Delete the Render service, BigQuery proof table/dataset as approved, service-account key/account, and local key; then repeat this judgment.
