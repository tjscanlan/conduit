<!-- Generated from proto/*.proto and internal/schema by scripts/gen-api-docs.py. Do not edit. -->

# API reference

Agents talk to the gateway over gRPC with mutual TLS on port 4317, using the `IngestionService` in package `observability.v1`. The SDKs wrap this API. Use it directly only if you're writing a client in another language. This page is generated from the `.proto` files and the gateway's validation constants, so it always matches the code.

## Authentication and identity

Every RPC requires a TLS 1.3 client certificate signed by the CA the gateway trusts. The certificate's CommonName is the agent's identity. The gateway stamps it on every span (`agent_name`), log (`agent_name`) and metric (`agent` label), overwriting any value the client supplies.

## Errors and partial acceptance

- **`IngestSpan`** returns `accepted: false` with a `rejection_reason` when validation fails.
- **`IngestBatch`** validates each item independently. It returns per-kind accepted counts, plus an `errors` list with entries like `log[2]: validation: log body is required`, where the index is the item's position in its kind's list.
- **gRPC status `RESOURCE_EXHAUSTED`:** the agent is over its rate limit (charged per item). Retry later.
- **gRPC status `INVALID_ARGUMENT`:** the batch is larger than 1000 items or the per-agent limit; split it.
- **gRPC status `UNAUTHENTICATED`:** no valid client certificate.

## Ingestion limits

| Applies to | Field | Rule |
|---|---|---|
| Span | `trace_id`, `span_id`, `parent_id` | 1–64 characters of `[A-Za-z0-9_-]` (`parent_id` may be empty). SDKs send W3C sizes: 32 and 16 hex characters. |
| Span | `name` | Required, at most 256 bytes |
| Span | duration | `end_time` ≥ `start_time`, at most 24 hours |
| Span, log | `attributes` | At most 128 entries; keys 1–128 bytes; values at most 4 KiB |
| Span | `llm_call.prompt` / `completion` | At most 128 KiB each (dropped at the gateway when `STORE_LLM_PAYLOADS=false`) |
| Span | `llm_call.model` | At most 256 bytes |
| Span | `llm_call` tokens and cost | Token counts ≥ 0; `cost_usd` finite and ≥ 0 |
| Log | `body` | Required, at most 32 KiB |
| Log, metric | `timestamp` | No more than 1 hour in the future; the receive time if unset |
| Log | `trace_id` / `span_id` | Optional, same format as spans; `span_id` requires `trace_id` |
| Metric | `name` | Prometheus name `[a-zA-Z_:][a-zA-Z0-9_:]*`, at most 200 characters; `conduit_*` and `__*` are reserved |
| Metric | `labels` | At most 32; keys `[a-zA-Z_][a-zA-Z0-9_]*` (at most 128, not `__*`); values at most 1 KiB. The `agent` label is always set by the gateway. |
| Metric | `value` | Finite; counters ≥ 0 |
| Batch | items per `IngestBatch` | At most 1000 (spans + metrics + logs), and no more than the per-agent rate limit (`RATE_LIMIT_RPS`) |

## `ingestion.proto`

### service `IngestionService`

IngestionService is the gateway's inbound API. All RPCs require mTLS — agent identity comes from the client certificate.

| RPC | Request | Response | Description |
|---|---|---|---|
| `IngestSpan` | `IngestSpanRequest` | `IngestSpanResponse` | IngestSpan submits a single span. Used for real-time streaming from agents. |
| `IngestBatch` | `IngestBatchRequest` | `IngestBatchResponse` | IngestBatch submits spans, metrics, and logs. Each item is validated and accepted independently (not atomically); rejected items are reported in IngestBatchResponse.errors. |

### message `IngestSpanRequest`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `span` | `Span` |  |

### message `IngestSpanResponse`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `accepted` | `bool` |  |
| 2 | `rejection_reason` | `string` | set if accepted=false |

### message `IngestBatchRequest`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `spans` | `repeated Span` |  |
| 2 | `metrics` | `repeated Metric` |  |
| 3 | `logs` | `repeated LogRecord` |  |

### message `IngestBatchResponse`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `spans_accepted` | `int32` |  |
| 2 | `metrics_accepted` | `int32` |  |
| 3 | `logs_accepted` | `int32` |  |
| 4 | `errors` | `repeated string` | one per rejected item, e.g. "log[2]: validation: ..." |

## `span.proto`

### message `Span`

Span represents a single unit of work in an agent execution trace.

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `trace_id` | `string` |  |
| 2 | `span_id` | `string` |  |
| 3 | `parent_id` | `string` | empty if root span |
| 4 | `name` | `string` |  |
| 5 | `start_time` | `google.protobuf.Timestamp` |  |
| 6 | `end_time` | `google.protobuf.Timestamp` |  |
| 7 | `status` | `SpanStatus` |  |
| 8 | `attributes` | `map<string, string>` |  |
| 9 | `llm_call` | `LLMCallData` | Optional — set only for LLM call spans |

### enum `SpanStatus`

| Value | Number | Description |
|---|---|---|
| `SPAN_STATUS_UNSET` | 0 |  |
| `SPAN_STATUS_OK` | 1 |  |
| `SPAN_STATUS_ERROR` | 2 |  |

### message `LLMCallData`

LLMCallData captures the full context of a single inference call. Stored separately from span metadata to allow payload-level access control.

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `model` | `string` |  |
| 2 | `prompt` | `string` |  |
| 3 | `completion` | `string` |  |
| 4 | `input_tokens` | `int32` |  |
| 5 | `output_tokens` | `int32` |  |
| 6 | `cost_usd` | `double` |  |

## `log.proto`

### message `LogRecord`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `timestamp` | `google.protobuf.Timestamp` |  |
| 2 | `severity` | `LogSeverity` |  |
| 3 | `body` | `string` |  |
| 4 | `attributes` | `map<string, string>` |  |
| 5 | `trace_id` | `string` |  |
| 6 | `span_id` | `string` |  |

### enum `LogSeverity`

| Value | Number | Description |
|---|---|---|
| `LOG_SEVERITY_UNSPECIFIED` | 0 |  |
| `LOG_SEVERITY_DEBUG` | 1 |  |
| `LOG_SEVERITY_INFO` | 2 |  |
| `LOG_SEVERITY_WARN` | 3 |  |
| `LOG_SEVERITY_ERROR` | 4 |  |

## `metric.proto`

### message `Metric`

| # | Field | Type | Description |
|---|---|---|---|
| 1 | `name` | `string` |  |
| 2 | `labels` | `map<string, string>` |  |
| 3 | `value` | `double` |  |
| 4 | `timestamp` | `google.protobuf.Timestamp` |  |
| 5 | `type` | `MetricType` |  |

### enum `MetricType`

| Value | Number | Description |
|---|---|---|
| `METRIC_TYPE_UNSPECIFIED` | 0 |  |
| `METRIC_TYPE_GAUGE` | 1 |  |
| `METRIC_TYPE_COUNTER` | 2 |  |
