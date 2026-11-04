# Changelog

All notable changes to Conduit are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
the [versioning policy](https://tjscanlan.github.io/conduit/releasing.html#versioning-policy).

## [Unreleased]

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

[Unreleased]: https://github.com/tjscanlan/conduit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tjscanlan/conduit/releases/tag/v0.1.0
