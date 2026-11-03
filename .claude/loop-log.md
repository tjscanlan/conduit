# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

## 2026-11-03 — Loop set up
- Did: created `loop/auto` branch, `.claude/loop.md`, and this log.
- Verified: baseline build FAILS (cmd/gateway: undefined RegisterIngestionServiceServer — proto not generated; protoc/buf not installed). No test files anywhere.
- Next candidates: generate proto types reproducibly; add tests for internal/schema and internal/auth.
