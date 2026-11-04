# AGENTS.md

Instructions for AI coding agents working in this repository. Human contributors should read [docs/contributing.md](docs/contributing.md), which covers the same ground in more detail.

## What this is

Conduit is secure, self-hosted observability for AI agents. Agents send spans, logs and metrics over gRPC with mTLS, and these services handle them:

| Service | Path | Role |
|---------|------|------|
| gateway | `cmd/gateway` | Authenticates agents (client certificate CommonName = agent identity), rate-limits, validates, and publishes to NATS JetStream (`SPANS`, `LOGS`, `METRICS` streams) |
| collector | `cmd/collector` | Consumes NATS. Writes ClickHouse (spans, logs) and VictoriaMetrics (metrics; span durations best-effort) and Tempo (best-effort). Owns the schema: runs migrations, reconciles retention |
| ui | `cmd/ui` | Server-rendered UI (`html/template` + htmx), reading ClickHouse only |
| SDKs | `sdk/go` (separate Go module), `sdk/python` (`conduit_sdk`) | Client libraries |

Docs: `docs/`, published at https://tjscanlan.github.io/conduit/. The machine-readable index is https://tjscanlan.github.io/conduit/llms.txt.

## Commands

```bash
go build ./... && go vet ./... && go test -race ./...    # root module
(cd sdk/go && go test -race ./...)                       # Go SDK (a separate module)
(cd sdk/python && python -m pytest)                      # Python SDK (pip install -e '.[dev]' first)
gofmt -l .                                               # must print nothing
make proto        # after editing proto/*.proto; commit the generated Go and Python code
make roadmap      # after editing docs/_data/roadmap.json; commit docs/roadmap.md
python3 scripts/build-agent-docs.py --check              # docs manifest is complete
```

End-to-end check (needs Docker): start the stack as in [docs/deployment.md](docs/deployment.md), then run `scripts/smoke-test.sh`. CI runs it on every push.

## Invariants: never break these

- **Agent identity comes only from the mTLS certificate.** The gateway sets `agent_name` (spans and logs) and the `agent` metric label from the certificate's CommonName and overwrites any client-supplied value. Never trust an identity from the payload.
- **Validate at the gateway, before publishing.** New telemetry fields need limits in `internal/schema`, plus rejection-path tests.
- **Metric names starting with `conduit_` or `__` are reserved** for Conduit's own series.
- **Prometheus label values must be escaped** (backslash, quote and newline) in `internal/storage/victoriametrics.go`. Otherwise one agent can inject samples for another.
- **ClickHouse queries use bound parameters,** never string interpolation of input. The only exceptions are validated integers and constant identifiers.
- **Bind times as integers.** Positional binding of a `time.Time` truncates it to **whole seconds**. Pass Unix seconds or nanoseconds, and insert with `PrepareBatch`/`Append`.
- **Schema changes go in a new migration,** `internal/storage/migrations/NNNN_name.sql`: one idempotent statement (`IF NOT EXISTS` / `IF EXISTS`). Never edit a shipped migration; checksums are enforced at startup.
- **UI: no inline scripts or styles.** The CSP forbids them, and `TestTemplatesHaveNoInlineScript` enforces it. Build links that carry query strings as `template.URL` in Go; `html/template` re-encodes queries interpolated into an `href`.
- **UI colors follow [docs/brand.md](docs/brand.md).** Only `#ec0000` may be text on white, and `#ec9b00`/`#ecca00` never carry white text.
- **Generated code is committed and drift-checked.** That covers `proto/*.pb.go`, `sdk/python/src/conduit_sdk/_proto/` and `docs/roadmap.md`. Regenerate it; don't hand-edit it.
- **Few dependencies.** The project's pitch includes "no supply-chain surprises". Justify any new module in the PR, pin GitHub Actions by commit SHA, and pin CDN assets with SRI.

## Making a change

1. Branch from `main`. Keep each change to one theme.
2. Add or extend tests for the behavior you change, including failure paths.
3. Update the matching doc in `docs/` in the same change. A new doc file must also be added to `docs/_data/docs.json`, which drives the site nav and `llms.txt`.
4. Run the checks above. CI must be green: `test`, `generated`, `python-sdk`, `vuln`, `docker`, `smoke`, and the `docs site` workflow.
5. Commit messages use the form `area: what and why`.

## Where things are

| Need | Look in |
|------|---------|
| Validation rules and limits | `internal/schema/` |
| NATS publish and consume, delivery semantics | `internal/storage/nats.go`, `internal/telemetry/collector.go` |
| ClickHouse queries and migrations | `internal/storage/clickhouse.go`, `internal/storage/migrations/` |
| Wire API | `proto/*.proto` (`IngestSpan`, `IngestBatch`) |
| UI handlers and templates | `cmd/ui/`, `ui/templates/`, `ui/static/` |
| Auth (mTLS, UI login, GCP IAP) | `internal/auth/`, `cmd/ui/auth.go` |
| Environment variables | [docs/deployment.md](docs/deployment.md), section "Configuration reference" |
| Automated work loop | `.claude/loop.md` (process), `.claude/loop-log.md` (history) |
