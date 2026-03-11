# Agent Observability Platform

A secure, runtime-safe observability component for AI agent platforms.
Built with Go, no npm, no JVM, no supply chain surprises.

## Architecture

```
Agent SDK (Go/Python)
    │  OTLP over mTLS/gRPC
    ▼
Ingestion Gateway  ──▶  NATS JetStream
                              │
                    ┌─────────┼──────────┐
                    ▼         ▼          ▼
               ClickHouse  VictoriaMetrics  Tempo
                    │         │          │
                    └─────────┴──────────┘
                              │
                         Query API + UI (Go + HTMX)
```

## Components

| Component | Description |
|-----------|-------------|
| `cmd/gateway` | Ingestion endpoint — mTLS, schema validation, rate limiting |
| `cmd/collector` | Pulls from NATS, fans out to storage backends |
| `cmd/ui` | Query API + server-rendered UI (HTMX) |
| `sdk/go` | Instrumentation SDK for Go agents |
| `sdk/python` | Instrumentation SDK for Python agents |

## Quick Start

```bash
# Generate mTLS certs
./scripts/gen-certs.sh

# Generate protobuf types
./scripts/gen-proto.sh

# Run full stack
docker compose -f deployments/docker/docker-compose.yml up
```

## Security Model

- All agent→gateway communication over mTLS (mutual TLS)
- Schema validation at ingestion boundary — malformed spans rejected before storage
- Rate limiting per agent identity
- Ingestion and query networks are separated
- Single static Go binary, deployable as `FROM scratch` container
- Zero external JS dependencies — UI assets embedded in binary

## Tech Stack

- **Language**: Go 1.22+
- **Wire format**: Protobuf / OTLP
- **Transport**: gRPC + mTLS
- **Message buffer**: NATS JetStream
- **Trace storage**: Grafana Tempo
- **Metrics storage**: VictoriaMetrics
- **LLM payload storage**: ClickHouse
- **UI**: HTMX + Go html/template (no npm, no bundler)

## Docs

- [Architecture](docs/architecture.md)
- [Security Model](docs/security.md)
- [Go SDK](docs/sdk-go.md)
- [Python SDK](docs/sdk-python.md)
- [Deployment](docs/deployment.md)
- [Contributing](docs/contributing.md)
