# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

## 2026-11-03 — Fix rate limiting: was global, now per-agent
- Did: the interceptor chain ran the rate limiter before auth, so no identity was in context yet and every agent shared one bucket keyed "". A single noisy agent could starve all the others. I moved the chain into `unaryInterceptors()` with auth first, added `auth.WithIdentity`, and made the gateway fail fast when the CA PEM doesn't parse (it used to ignore the result silently). Added tests: per-agent buckets, and auth running before the limiter.
- Verified: go build, go vet, and go test -race ./... PASS.
- Next candidates: harden `schema.Validate` (no length limits on trace_id, span_id, or model; attribute keys can be empty; cost_usd can be NaN or negative; no UTF-8 check) and add its tests; the rate-limiter `sync.Map` never evicts buckets (bounded by distinct certificate CNs, so low priority); wire the Go SDK client TODO.
- Blockers / needs human: none new.

## 2026-11-03 — Generate proto code; gateway compiles
- Did: rewrote `scripts/gen-proto.sh` to need only Go (pinned buf v1.47.2, protoc-gen-go v1.34.1, protoc-gen-go-grpc v1.3.0 installed into bin/tools); added `proto/buf.yaml` and `proto/buf.gen.yaml`; generated `proto/*.pb.go` (still gitignored by design, so run `make proto` after cloning). Implemented `IngestSpan` and `IngestBatch` on `IngestionHandler` plus the `spanInputFromProto` conversion, and added tests for it. `bin/` is now gitignored.
- Verified: go build, go vet, and go test -race ./... all PASS (first green build).
- Next candidates: wire `sdk/go/sdk.go` to the generated client (the TODO at line ~112); run gofmt on pre-existing files (schema/validator.go, storage/clickhouse.go, telemetry/processor.go, sdk/go/llm.go); write tests for internal/schema validation and the rate limiter. SpanStatus is dropped at conversion because SpanInput has no field for it.
- Blockers / needs human: IngestBatch rejects metrics and logs because there's no pipeline for them yet. The proto says "atomically", but spans are accepted one at a time. Decide which semantics you want.

## 2026-11-03 — Loop set up
- Did: created `loop/auto` branch, `.claude/loop.md`, and this log.
- Verified: baseline build FAILS (cmd/gateway: undefined RegisterIngestionServiceServer — proto not generated; protoc/buf not installed). No test files anywhere.
- Next candidates: generate proto types reproducibly; add tests for internal/schema and internal/auth.
