# Changelog

All notable changes to Conduit are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
the [versioning policy](https://tjscanlan.github.io/conduit/releasing.html#versioning-policy).

## [Unreleased]

## [0.2.0]

Activation: from a clean machine with only Docker to your own agent's first
trace in under five minutes.

### Added

- **`conduitctl`:** a CLI, released as signed binaries for Linux, macOS and
  Windows. `conduitctl quickstart` runs a local stack from the released images
  with no checkout. `agent-cert NAME` issues more agent certificates, and
  `doctor` diagnoses Docker, ports, service health, the UI and the gateway.
  There are also `status`, `down` and `reset`.
- **One-command quickstart:** `make quickstart` / `deployments/quickstart/quickstart.sh`.
  A `conduit-init` container generates a dev CA, the gateway and agent
  certificates, and a UI login. ClickHouse is tuned to fit a 2 GiB Docker VM,
  and a simulated `demo-agent` sends traces from the start. CI checks that a
  cold start reaches the first trace within 300 seconds.
- **Get started page in the UI** (`/start`): a live walkthrough from a running
  stack to your agent's first trace. Certificates are made on the command line
  only; the UI never serves keys. New settings: `UI_GATEWAY_ADDR`,
  `UI_DEMO_AGENT`.
- **Examples** (`examples/`): hello-trace, a tool-calling agent, an asyncio
  agent (Python), and Claude and OpenAI agents on the official SDKs. All run in
  CI, the provider examples against a fake API, so no keys are needed.
- **SDK setup from the environment:** `sdk.NewFromEnv()` (Go) and
  `Client.from_env()` / `conduit.configure()` (Python) read `CONDUIT_CERT_DIR`,
  `CONDUIT_AGENT` and `CONDUIT_GATEWAY_ADDR`. `Check()` / `check()` verify the
  gateway accepts the agent at startup. Errors say what to fix: a missing or
  mismatched certificate file, the gateway's server certificate used as an
  agent's, the wrong CA, a host not in the gateway certificate, the gateway not
  running, rate limits.
- **Python SDK:** `@traced` for sync and async functions; module-level
  `span()`, `current_span()` and `shutdown()`; `llm_call_from_anthropic` and
  `llm_call_from_openai` for real token usage and cost; optional `[anthropic]`
  and `[openai]` extras.
- **Release artifacts:** `ghcr.io/tjscanlan/conduit-init` and
  `conduit-demo-agent` images, and `conduitctl` archives with a cosign-signed
  `SHA256SUMS`.

### Changed

- **Python:** `Client(...)` raises `ConfigError` (a `ConduitError`) for missing,
  mismatched or non-PEM certificate files, where it used to raise
  `FileNotFoundError` or fail later. `Client.from_env()` also accepts the 0.1.0
  variable names `TLS_CERT`, `TLS_KEY` and `TLS_CA`.
- **Go:** `sdk.New` rejects a certificate without client-auth usage (such as the
  gateway's `server.crt`) and expired or not-yet-valid certificates up front.
  RPC errors gain a hint, and the gRPC error stays wrapped.

### Security

- `golang.org/x/net` v0.62.0 (GO-2026-6866, GO-2026-6860) in every module.

## [0.1.0]

The first tagged release.

### Added

- **Ingestion:** gRPC gateway with mTLS agent identity (the certificate
  CommonName becomes `agent_name`), validation at ingestion, and per-agent rate
  limiting charged per telemetry item.
- **Pipeline:** NATS JetStream work queues with at-least-once delivery into
  ClickHouse (spans and logs) and VictoriaMetrics (metrics), and best-effort
  export of span data to VictoriaMetrics and Tempo.
- **SDKs:** Go (`github.com/tjscanlan/conduit/sdk/go`) and Python
  (`conduit-observability` on PyPI), with spans, status and error recording,
  logs, gauges and counters.
- **UI:** login form with signed session cookies or Google Cloud IAP, a strict
  CSP, a trace list, trace detail with a span tree and waterfall, and a logs page.
- **Data governance:** `STORE_LLM_PAYLOADS=false` to drop prompts and completions
  at the gateway, `SPAN_RETENTION_DAYS`, and versioned, checksummed ClickHouse
  schema migrations.
- **Operations:** `/livez` and `/readyz` on every service, a `healthcheck`
  subcommand for scratch images, Docker Compose and Kubernetes manifests, and GKE
  IAP manifests.
- **Release artifacts:** multi-arch (amd64, arm64) images on GHCR, signed with
  cosign, with an SBOM and build provenance.

### Changed

- **Breaking, Go:** module paths moved from the `github.com/your-org/...`
  placeholders to `github.com/tjscanlan/conduit` and
  `github.com/tjscanlan/conduit/sdk/go`. Update your imports.
- **Breaking, Python:** the distribution `conduit-sdk` is now
  `conduit-observability`, and the import `conduit_sdk` is now
  `conduit_observability`. The name `conduit-sdk` on PyPI belongs to an unrelated
  project, so remove any checkout-based install and update your imports.
- Kubernetes manifests pin images to the release tag instead of `:latest`.

[Unreleased]: https://github.com/tjscanlan/conduit/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/tjscanlan/conduit/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/tjscanlan/conduit/releases/tag/v0.1.0
