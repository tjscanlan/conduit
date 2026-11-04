---
layout: roadmap
title: Roadmap
hide_title: true
body_class: roadmap-page
---

<!-- Generated from docs/_data/roadmap.json by scripts/gen-roadmap.py. Do not edit. -->

# Roadmap

Conduit's goal is to be the observability layer a security-conscious team can run next to its AI agents. It should be self-hosted, small enough to audit, and focused on what matters for agents: steps, tools, models, errors and cost.

This roadmap is ordered by priority, not by date. Items move as we learn. To suggest a change, open an issue or a PR against `docs/_data/roadmap.json`.

## Shipped

**Foundation**
- mTLS agent identity, with the certificate CommonName as `agent_name`
- Span validation at ingestion
- Per-agent rate limiting, charged per telemetry item
- NATS JetStream work queue with at-least-once delivery into ClickHouse
- Best-effort export to VictoriaMetrics and Tempo
- Span status (ok, error, unset) from end to end, and nanosecond timestamps

**SDKs**
- Go and Python SDKs: mTLS that trusts only your CA, W3C-sized IDs, status and error recording
- Python context that is safe across threads and asyncio tasks

**UI**
- Sign-in: a login form with signed session cookies, or Google Cloud IAP
- Strict CSP and browser hardening
- Trace list: one row per trace, filters by time, agent and errors, keyset paging, agent suggestions
- Trace detail: span tree, waterfall timeline, error and LLM details, deep links

**Data governance**
- `STORE_LLM_PAYLOADS=false` drops prompts and completions at the gateway
- `SPAN_RETENTION_DAYS`, with the TTL reconciled on existing tables

**Operations**
- `/livez` and `/readyz` on every service, and a `healthcheck` subcommand for scratch images
- Compose restart policies and Kubernetes probes, plus GKE IAP manifests

**Quality**
- CI: race tests, a generated-code drift check, govulncheck, Docker builds, and a full-stack smoke test with both SDKs
- Brand identity, documentation set, and this docs site

## Now: hardening for first production users

| Item | Area | Why |
|------|------|-----|
| **Prompt and completion access control** | UI | Payloads are stored but never shown. Showing them needs a decision on who may see them (see [open decisions](#open-decisions)). |
| **Schema migrations** | Storage | Today `CREATE TABLE IF NOT EXISTS` runs at startup. Versioned migrations are needed before the schema changes again. |
| **Metrics and logs ingestion** | Pipeline | `IngestBatch` accepts metrics and logs in the API but rejects them, because there's no pipeline yet. |

## Next: scale and integration

| Item | Area | Why |
|------|------|-----|
| **Batched, asynchronous SDK delivery** | SDKs | One blocking RPC per span is simple but slow for chatty agents. Add a buffered exporter with backpressure. |
| **Read the trace list from `trace_summary`** | Storage | Avoid aggregating raw spans for every list query at high volume. Needs root names and agents in the view. |
| **OpenTelemetry interoperability** | Pipeline | Accept OTLP from existing OpenTelemetry agents, and use OTel semantic conventions for LLM attributes. |
| **Search** | UI | Search attributes, error messages and span names across traces. |
| **Published SDKs** | SDKs | A PyPI package and a tagged Go module with stable import paths (see open decisions). |
| **Helm chart** | Operations | Package the Kubernetes manifests with sensible defaults. |

## Later: insight

| Item | Area | Why |
|------|------|-----|
| **Cost and token dashboards** | UI | Spend by agent, model and time, built on the existing rollups. |
| **Alerts** | Operations | Error-rate and latency thresholds per agent, with webhook delivery. |
| **Trace comparison** | UI | Compare two runs of the same task side by side. |
| **Multi-tenancy** | Security | Separate projects or teams with scoped UI access. |

## Open decisions

These need a maintainer's call before the related work can start.

| Decision | Options | Raised in |
|----------|---------|-----------|
| **Who can see prompts and completions** | Every signed-in user; A separate role or permission; Reveal-on-click with an audit log | [PR #3](https://github.com/tjscanlan/conduit/pull/3) |
| **Publishing the SDKs** | Keep them repo-local; Publish to PyPI and as a tagged Go module (needs real module paths instead of `github.com/your-org/...`) | [PR #2](https://github.com/tjscanlan/conduit/pull/2) |
| **`nats-server` as a test dependency** | Add an in-process NATS for integration tests; Rely on the compose smoke test, which already covers the path. Leaning toward the smoke test. | [PR #2](https://github.com/tjscanlan/conduit/pull/2) |
