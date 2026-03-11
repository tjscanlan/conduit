# Architecture

## Overview

This system collects, stores, and surfaces observability data from AI agents.
It is designed for **runtime safety** (single static binaries, no JVM) and **security**
(mTLS everywhere, schema validation at the boundary, minimal attack surface).

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│  Agent Process                                                      │
│  ┌────────────────────────────────────┐                             │
│  │  SDK (Go or Python)                │                             │
│  │  - wraps OTLP protobuf             │                             │
│  │  - holds client mTLS cert          │                             │
│  └────────────────┬───────────────────┘                             │
└───────────────────┼─────────────────────────────────────────────────┘
                    │ gRPC + mTLS (port 4317)
                    ▼
┌───────────────────────────────────────────────────────────────────┐
│  Ingestion Gateway (cmd/gateway)                                  │
│  - verifies client cert (mutual TLS)                              │
│  - extracts agent identity from CN                                │
│  - validates span schema (rejects malformed/oversized payloads)   │
│  - rate limits per agent identity                                 │
│  - publishes to NATS JetStream                                    │
└──────────────────────────┬────────────────────────────────────────┘
                           │ NATS JetStream (in-process or remote)
                           ▼
┌───────────────────────────────────────────────────────────────────┐
│  Collector (cmd/collector)                                        │
│  - subscribes to NATS subjects                                    │
│  - fans out to storage backends                                   │
│  - handles retries and backpressure                               │
└──────────┬────────────────┬────────────────────────────────────────┘
           │                │                          │
           ▼                ▼                          ▼
     ClickHouse      VictoriaMetrics              Grafana Tempo
  (spans + LLM        (counters,                  (trace graph,
   payloads)          histograms)                  waterfall)
           │                │                          │
           └────────────────┴──────────────────────────┘
                            │
                            ▼
┌──────────────────────────────────────────────────────┐
│  Query API + UI (cmd/ui)                             │
│  - Go http.Server                                    │
│  - html/template + HTMX (no npm, no bundler)         │
│  - assets embedded via embed.FS                      │
└──────────────────────────────────────────────────────┘
```

## Security Boundaries

### mTLS
Every agent→gateway connection uses mutual TLS (TLS 1.3 minimum).
The agent presents a client certificate signed by the internal CA.
The gateway verifies it before the gRPC handshake completes.
Agent identity is extracted from `Subject.CommonName` of the verified cert.

### Schema Validation
All inbound spans are validated at the gateway before touching NATS or any storage.
Oversized payloads, malformed trace IDs, and invalid timestamps are rejected with a
gRPC error. This prevents garbage or hostile data from reaching ClickHouse.

### Network Separation
- **Ingestion port (4317)**: gRPC, mTLS, accessible from agent networks only
- **UI/query port (8080)**: HTTP, accessible from internal tooling networks only
- Storage backends (ClickHouse, NATS) are not exposed outside the internal network

### Container
Production images are built `FROM scratch` — no shell, no package manager, no OS utilities.
The only file in the container is the static Go binary and the CA certificates bundle.

## Storage Schema

### ClickHouse `spans` table

| Column | Type | Notes |
|--------|------|-------|
| trace_id | String | Groups spans into a trace |
| span_id | String | Unique per span |
| parent_id | String | Empty for root spans |
| agent_name | String | From mTLS CN |
| span_name | String | e.g. "llm-call", "tool:search" |
| start_time | DateTime64(9) | Nanosecond precision |
| duration_ms | UInt64 | Derived from end-start |
| status | Enum8 | ok / error / unset |
| attributes | Map(String,String) | Arbitrary key-value |
| llm_model | Nullable(String) | e.g. "claude-sonnet-4-20250514" |
| llm_prompt | Nullable(String) | Full prompt text |
| llm_completion | Nullable(String) | Full response text |
| llm_input_tokens | Nullable(UInt32) | |
| llm_output_tokens | Nullable(UInt32) | |
| llm_cost_usd | Nullable(Float64) | |

Partitioned by `toYYYYMMDD(start_time)`, ordered by `(agent_name, trace_id, start_time)`.
90-day TTL by default (configurable).

## SDK Interface

Both the Go and Python SDKs expose the same conceptual interface:

```
Client.span(name)          → context manager / returns Span
Span.set_attr(key, value)  → fluent
Span.record_llm_call(...)  → attach LLM metadata
Span.end()                 → flush to gateway
```

LLM prompt/completion content is transmitted only if `OTEL_INCLUDE_PAYLOADS=true`
is set — this lets operators opt out of storing sensitive prompt data.
