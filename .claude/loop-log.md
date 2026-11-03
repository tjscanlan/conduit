# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

## 2026-11-03 — Generate proto code; gateway compiles
- Did: rewrote `scripts/gen-proto.sh` to need only Go (pinned buf v1.47.2, protoc-gen-go v1.34.1, protoc-gen-go-grpc v1.3.0 installed into bin/tools); added `proto/buf.yaml` and `proto/buf.gen.yaml`; committed generated `proto/*.pb.go`. Implemented `IngestSpan` and `IngestBatch` on `IngestionHandler` plus the `spanInputFromProto` conversion, and added tests for it. `bin/` is now gitignored.
- Verified: go build, go vet, and go test -race ./... all PASS (first green build).
- Next candidates: wire `sdk/go/sdk.go` to the generated client (the TODO at line ~112); run gofmt on pre-existing files (schema/validator.go, storage/clickhouse.go, telemetry/processor.go, sdk/go/llm.go); write tests for internal/schema validation and the rate limiter. SpanStatus is dropped at conversion because SpanInput has no field for it.
- Blockers / needs human: IngestBatch rejects metrics and logs because there's no pipeline for them yet. The proto says "atomically", but spans are accepted one at a time. Decide which semantics you want.

## 2026-11-03 — Loop set up
- Did: created `loop/auto` branch, `.claude/loop.md`, and this log.
- Verified: baseline build FAILS (cmd/gateway: undefined RegisterIngestionServiceServer — proto not generated; protoc/buf not installed). No test files anywhere.
- Next candidates: generate proto types reproducibly; add tests for internal/schema and internal/auth.
