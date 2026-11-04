# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

## 2026-11-04 — Activation cycle 3: SDK quick start (`loop/sdk-quickstart`)
- **Go SDK:**
  - `NewFromEnv`/`ConfigFromEnv` read CONDUIT_CERT_DIR plus CONDUIT_AGENT (auto-picked when the dir holds exactly one clientAuth cert; otherwise the error lists them), CONDUIT_TLS_CERT/KEY/CA overrides, and CONDUIT_GATEWAY_ADDR (default localhost:4317; OTEL_GATEWAY_ADDR accepted).
  - `Client.Check(ctx)` sends an empty IngestBatch.
  - `diagnose()` wraps every RPC error with a hint: unknown CA, host not in SANs, client cert rejected, expired, connection refused, not TLS, rate limited, deadline, unavailable. The gRPC error stays wrapped.
  - `New` catches at construction: missing file, key mismatch, server.crt used as an agent (no clientAuth EKU), expired / not-yet-valid / no CN.
  - Flaky case found and fixed: under TLS 1.3, a gateway rejecting the client cert often surfaces as a bare broken pipe. New now records whether the agent cert chains to TLSCACert and uses that for the hint. 15/15 repeated runs pass.
- **Python SDK:**
  - `Client.from_env` uses the same variables (0.1.0's TLS_CERT/KEY/CA still accepted); `client.check()`; `ConfigError` raised at construction for a missing file, key mismatch (via `ssl.load_cert_chain`) or non-PEM CA; diagnosed RPC errors.
  - New `tracing` module: `configure()`, `span()`, `@traced` (sync and async; OK/error status; attrs), `current_span()`, `get_client()`, `shutdown()`. Unconfigured use is a no-op with one warning.
  - New `providers` module: `llm_call_from_anthropic` and `llm_call_from_openai` (Chat Completions and Responses), duck-typed, plus `[anthropic]` and `[openai]` extras.
  - 11 new tests, 21 total, passing on 3.12 and 3.14.
- **Examples:** the Go helper is now just "fill CONDUIT_CERT_DIR for the quickstart, then NewFromEnv plus Check". Python examples use `configure()`, `@traced` tools and the provider helpers. examples-check passes all 10 again.
- **Pre-release install:** the examples need the in-repo SDK (newer than PyPI 0.1.0), so the docs, banner and CI use `pip install -e "sdk/python[...]"`. The requirements.txt was dropped (its relative `-e` path only resolved from one directory). Go docs say `go get ...sdk/go@main` until v0.2.0. **Next release:** switch back to `pip install conduit-observability`.
- **Docs:** sdk-go.md and sdk-python.md now open with "Quick start (2 minutes)", Configuration (env table) and "When it doesn't connect". API tables updated; the PyPI README quick start uses the module API.
- **Diagnostics checked against the real quickstart gateway:** gateway down, server.crt as agent, several agents, unroutable host.
- **Deps:** golang.org/x/net v0.61.0 → v0.62.0 in all three modules (GO-2026-6866/6860). govulncheck: sdk/go and examples/go are clean; root has only GO-2026-5932 (x/crypto/openpgp, unmaintained, not called, no fix).

## 2026-11-04 — Activation cycle 2: prebuilt agent examples (`loop/agent-examples`)
- **`examples/go`:** a separate module, with replaces to the in-repo sdk/go and main module.
  - hello-trace, tool-agent (simulated model), claude-agent (anthropic-sdk-go v1.84.0) and openai-agent (openai-go/v3 v3.94.0, Chat Completions).
  - Shared `internal/quickstart` finds `deployments/quickstart/.conduit/certs` by searching upward; overridable with CONDUIT_GATEWAY_ADDR, CONDUIT_CERT_DIR and CONDUIT_AGENT. Shared `internal/weather` is a canned get_weather tool.
- **`examples/python`:** hello_trace, tool_agent, async_agent (asyncio.gather; spans nest per task and overlap, verified in ClickHouse), claude_agent (anthropic 0.125) and openai_agent (openai 2.48). `_quickstart.py` finds the certs; requirements.txt.
- **Shared design:**
  - Span layout: agent.run → llm.call (real usage, model from the response, cost estimate) → tool.<name> (tool.input, error status).
  - Tools run after the llm.call span ends. Caught and fixed in claude-agent, whose first version ran tools inside the model span.
  - Provider examples exit with a hint when no key is set.
  - Defaults: claude-sonnet-5 and gpt-5-mini, overridable with ANTHROPIC_MODEL / OPENAI_MODEL.
- **`scripts/fake-llm-server.py`:** stdlib fake of POST /v1/messages and /v1/chat/completions. Turn 1 requests get_weather for two cities; turn 2 must carry both tool results (else 400), then it answers. Both official SDKs honor ANTHROPIC_BASE_URL / OPENAI_BASE_URL, so the real request building and parsing run with no keys.
- **`scripts/examples-check.sh`:** runs all 9 examples against the quickstart plus the fake server, and checks each trace in the UI (my-agent plus the expected span names). It also checks the missing-key hint. Fixed a bug where `exit` inside the `if` loop killed the script. First CI run: all checks passed, but the job exited 143, because the EXIT trap's `wait` on the killed fake server fails under `set -e` with Linux bash (macOS bash 3.2 hid it). Fixed with `|| true`.
- **CI:**
  - The test job builds and vets examples/go.
  - The vuln job runs govulncheck on examples/go: passes; x/net v0.61.0 has GO-2026-6866/6860, but they aren't called.
  - The quickstart job now also installs the SDK plus requirements and runs examples-check.
- **Docs:** new docs/examples.md (in the manifest under SDKs, so it reaches llms.txt) and examples/README.md. The README, sdk-go.md and sdk-python.md link to it. The quickstart banner now says `go run ./hello-trace` / `python examples/python/hello_trace.py`.
- **Verified locally:** examples-check passes all 10 checks; go vet, gofmt and the docs manifest pass. Not verified: real Anthropic/OpenAI calls (no keys here).
- **Follow-up:** bump golang.org/x/net to v0.62.0 in all three modules (uncalled advisories).

## 2026-11-04 — Activation cycle 1: one-command quickstart (`loop/one-command-quickstart`)
- **`cmd/conduit-init`:** stdlib + x/crypto/bcrypt. Issues an ECDSA P-256 dev CA, a gateway cert (SANs gateway, localhost, 127.0.0.1), and client certs for demo-agent and my-agent. Also writes a random bcrypt UI login (ui-users, plus an owner-only ui-login.txt so the banner can reprint it). Idempotent: keeps valid files, reissues leaves if the CA is new, issues only newly added agents. Keys are 0600; runs as the host UID so `my-agent.key` is usable from the host. Tests: a real TLS 1.3 mTLS handshake with the generated files (both SANs); the UI's own LoadUsersFile/Authenticate; idempotency; perms; agent-name validation.
- **`deployments/quickstart/`:**
  - compose.yml pulls `ghcr.io/tjscanlan/conduit-*:v0.1.0` (`CONDUIT_VERSION`) and builds only init and demo-agent, from small Dockerfiles with no BuildKit-only features. Colima without buildx uses the classic builder, which rejected `RUN --mount`; caught locally.
  - Only UI and gateway are exposed, on 127.0.0.1, with overridable ports.
  - ClickHouse runs a low-memory profile with system logs off (~375 MiB idle); 7-day retention.
  - quickstart.sh (POSIX sh) does preflight, writes a 0600 .env of random secrets, then `up --wait`, waits for the first demo-agent span, and prints the banner. It also has `status`, `down` and `reset`. `make quickstart` / `make quickstart-down`.
- **`sdk/go/examples/demo-agent`:** simulated plan → tools → answer runs every 8s, with LLM tokens/cost, a ~1-in-12 tool failure after a logged retry, and gauge + counter. Every span has demo=true.
- **CI:** `quickstart` job runs scripts/quickstart-check.sh. It does a cold quickstart within a 300s budget (written to the step summary), a UI login with the generated credentials, demo-agent on /traces, then a my-agent trace sent with the generated certs, visible at /traces/{id}.
- **Verified locally** (Colima 2 CPU/2 GiB, alongside the user's untouched conduit-demo stack): healthy plus first trace in 38s (images cached, builds cold), 12s after `reset`, and 1–2s on rerun. The check script passes. Stack stopped afterward.
- **Docs:** README quick start is now `make quickstart`; docs/deployment.md has a "Local: quickstart" section; AGENTS.md mentions the check.
- **Follow-ups:**
  - Publish conduit-init and demo-agent images in release.yml (cycle 5, with conduitctl), so the quickstart builds nothing.
  - The banner's SDK snippet gets simpler once cycle 3 adds from_env / CONDUIT_CERT_DIR.

## 2026-11-04 — **Activation program (user-approved plan):** first trace in under 5 minutes
- Goal: from a clean machine with only Docker, a first trace is visible in the UI in under 5 minutes, measured in CI. Remove deployment and instrumentation friction.
- Baseline friction: clone; gen-certs (OpenSSL); UI password (needs Go); export CLICKHOUSE_PASSWORD; `compose up --build` builds 3 images from source (slow, OOMs a 2 GiB VM); wire 3 cert paths into SDK code. About 6 steps and 3 prerequisites.
- **User decisions:** quickstart via compose + init container **and** a `conduitctl` CLI (cycle 5); a **demo agent on by default** in the quickstart (labeled `demo-agent`, stoppable in one command, never in production manifests); examples with **simulated responses plus Claude and OpenAI** (key-gated; provider SDKs are optional extras); the walkthrough gives agent certs via a **CLI command only** (the UI never serves keys).
- **Cycles (each ends in a PR; stop after each):**
  1. `loop/one-command-quickstart`: deployments/quickstart compose pulling the signed GHCR v0.1.0 images; a `conduit-init` container generating CA, gateway, agent and UI credentials into a volume; `make quickstart`; ClickHouse tuned for 2 GiB; the demo agent; a printed next-steps banner; a CI time-to-first-trace job with a budget.
  2. `loop/agent-examples`: examples/ in Go and Python (hello-trace, tool-calling, async; Claude and OpenAI agents gated on API keys); CI runs the simulated modes.
  3. `loop/sdk-quickstart`: from_env/NewFromEnv with CONDUIT_CERT_DIR; Python `@traced` and module-level configure/span; Anthropic and OpenAI usage helpers as optional extras; actionable first-run error diagnostics; SDK docs rewritten around a 2-minute path.
  4. `loop/onboarding-walkthrough`: an in-UI "Get started" page (status checklist, snippets, live "waiting for your first trace", celebration link, next steps), shown only while there's no data and dismissible.
  5. `loop/conduitctl`: a release-shipped CLI (`quickstart`, `doctor`, `agent-cert`) wrapping the compose flow.

## 2026-11-04 — Release PR opened
- PR #11 merged. Opened https://github.com/tjscanlan/conduit/pull/12; no new work until it merges.
- The user added the PyPI trusted publisher, so 0.1.0 goes to PyPI through `release.yml` rather than a manual twine upload, with no tokens and no TestPyPI dry run. `~/.pypirc` holds placeholders only.
- **User authorized pushing the tags** (overriding "the loop never pushes tags" for this release): after the merge, tag the merge commit `v0.1.0` and `sdk/go/v0.1.0` and push atomically.
- Blockers / needs human: merge the PR.

## 2026-11-04 — Release tooling: module paths, release.yml, CHANGELOG, release docs
- User: "do what needs to be done". `~/.pypirc` was still missing (checked twice), so the uploads stayed blocked and I did the independent release work.
- Did:
  - **Module paths:** `github.com/tjscanlan/conduit` and `github.com/tjscanlan/conduit/sdk/go`; regenerated proto code. The SDK requires the root at v0.1.0; the `replace` stays for in-repo development and consumers ignore it. Removed the "Publishing the SDKs" open decision from the roadmap.
  - **`.github/workflows/release.yml`** (tag `v*`): verify, multi-arch GHCR images with SBOM, provenance and keyless cosign, PyPI via trusted publishing with `skip-existing` (0.1.0 is uploaded by hand first), consumer-style Go module check, GitHub Release from the CHANGELOG. All actions pinned by SHA. New `docker/setup-qemu-action` for arm64.
  - **`scripts/release-check.sh`** checks that the tag, Python version, SDK require, k8s image tags and CHANGELOG agree; CI runs it. `CHANGELOG.md`; `docs/releasing.md` (versioning policy, verifying, upgrading, cutting a release). K8s manifests pin `:v0.1.0` instead of `:latest`.
  - **Rejected:** `FROM --platform=$BUILDPLATFORM` to avoid QEMU. The legacy Docker builder, which is what this machine has (no buildx), fails on it, which would break local builds. QEMU is slower but only affects the release job.
- Verified: go build, vet, test -race in both modules; python 10/10; release-check happy path and failures (mismatch exit 1, bad tag exit 2); workflow YAML parses; plain `docker build` works locally. **Not verified:** actionlint (running it was denied as external code), buildx multi-arch, signing, PyPI publishing, and Go consumer resolution, all of which only run on a real tag.
- **`sdk/python/dist` was rebuilt** after the go_package change, so the earlier hashes are obsolete. Upload the rebuilt files.
- Next: TestPyPI then PyPI upload once `~/.pypirc` exists; open the release PR after #11 merges.
- Blockers / needs human: (1) create `~/.pypirc`; (2) add the trusted publisher on PyPI (see docs/releasing.md); (3) merge #11, then the release PR; (4) push the tags.

## 2026-11-04 — Cycle 12 (`loop/release-v0-1`, stacked on PR #11 by user request): Python SDK packaged for PyPI
- User: "I have my PyPI account set up. Use [packaging.python.org's packaging tutorial] to publish the Python SDK, then continue with this loop." PR #11 was still open, so I started the release branch **stacked on `loop/agent-readable-docs`** (it avoids conflicts in the regenerated stubs). Its PR goes to main after #11 merges.
- Did:
  - **Renamed** the dist `conduit-sdk` (taken on PyPI) to `conduit-observability`, and the import `conduit_sdk` to `conduit_observability` (git mv; all references in code, tests, codegen script, CI, docs, AGENTS.md, smoke). `conduit-sdk-go` was deliberately left alone; it's renamed later in this cycle.
  - pyproject per the guide: version **0.1.0**, SPDX `license = "Apache-2.0"` plus `license-files` (hatchling ≥1.27), classifiers (3.10–3.14, Alpha), keywords, project URLs (docs site, repo, issues), author name only (no email). LICENSE copied into sdk/python. A PyPI-ready README with absolute links.
  - **sdist bug caught by inspection:** unanchored `include` patterns (`LICENSE`, `tests/`) matched about 25 files inside `.venv-build/`. They're now anchored (`/src`, `/LICENSE`, …) and `.venv*` is excluded. Verified 0 venv files in the sdist.
- Verified: `python -m build` produced the sdist and a py3-none-any wheel; `twine check --strict` passed both; the wheel contains only the package, stubs, and license; the METADATA is complete. **Installed the built wheel in clean venvs on Python 3.10 and 3.14 and ran the suite against the installed package: 10/10 on each.** `make proto` regenerates byte-identical stubs at the new path.
- Next: upload to TestPyPI and verify an install, then PyPI (needs the user's token in `~/.pypirc`, never in chat). Then the rest of the release cycle: Go module paths, release.yml (GHCR multi-arch signed images, SBOM, provenance, PyPI trusted publishing for future releases), CHANGELOG, versioning policy, release docs.

## 2026-11-04 — **User directive for the NEXT cycle:** release v0.1.0 (`loop/release-v0-1`)
- The user asked: make the cycle right after the agent-docs PR merges a dedicated release cycle, and don't wait for more features (prompt access control, batched SDK delivery, OTel, and Helm go to v0.2+).
- **SDK publishing decision (user): publish both**, a tagged Go module and PyPI.
  - **PyPI name:** `conduit-sdk` is **taken** by an unrelated project (Conduit.app utils, v0.2.5), which likely also imports as `conduit_sdk`. Plan: distribution `conduit-observability` (free as of 2026-11-04) and a matching import `conduit_observability`. This is a breaking rename, acceptable pre-1.0; call it out in CHANGELOG and docs. The user may override the name.
  - Go: module paths `github.com/your-org/agent-observability` → `github.com/tjscanlan/conduit`, and `github.com/your-org/conduit-sdk-go` → `github.com/tjscanlan/conduit/sdk/go`, with tag `sdk/go/v0.1.0` (the nested module needs a prefixed tag). Remove the SDK's `replace` directive for the release, or keep it working with a go.work-based dev setup.
  - PyPI: GitHub OIDC **trusted publishing** (no tokens). **The user must create a pending publisher on PyPI** (project `conduit-observability`, repo tjscanlan/conduit, workflow `release.yml`, environment `pypi`). The loop can't do this; it goes in the PR checklist.
- Scope: real module paths everywhere (go.mod files, imports, generated `go_package`, docs); a tag-triggered `release.yml` (on `v*`) that builds multi-arch images (gateway, collector, ui) and pushes them to GHCR, signs them (cosign keyless, OIDC), attaches an SBOM (syft or docker buildx `--sbom`) and provenance, creates a GitHub Release with notes from the CHANGELOG, publishes the Python SDK to PyPI and verifies the Go module tag; CHANGELOG.md (Keep a Changelog), a versioning policy (SemVer, pre-1.0 caveats, what's API), and release docs (how to cut a release, verify signatures, upgrade).
- **Release itself:** the user merges the PR and pushes the `v0.1.0` (and `sdk/go/v0.1.0`) tags. The loop never pushes tags.

## 2026-11-04 — API reference generated from the source
- Did: `scripts/gen-api-docs.py` (stdlib) generates `docs/api.md` from **proto/*.proto** (services, RPCs, messages, field numbers and types, enums, and comments, via a small line parser that fails loudly on anything it can't parse) and the **validation constants** in internal/schema/*.go plus the gateway's maxBatchItems (it evaluates `N`, `a << b`, `N * time.Hour`, and strings, failing loudly otherwise). The page covers auth and identity, partial-acceptance errors (IngestBatch per-item `errors`, RESOURCE_EXHAUSTED, INVALID_ARGUMENT, UNAUTHENTICATED), and a 14-row limits table with exactly what the gateway enforces.
  - **Fixed a wrong proto comment found by documenting from source:** `IngestBatch` said "atomically", but it is per-item. Regenerated the Go and Python stubs.
  - Wired in: a "Reference" section in docs.json (nav, llms.txt, llms-full), `make api-docs` (also run by `make proto`), CI drift check on docs/api.md, the Pages page-exists list, and AGENTS.md and contributing.
- Verified: generator output is stable (rerun leaves no diff); manifest check OK; llms.txt lists the API reference with its description; builds pass. CI pending.
- **Stopping point:** the agent-docs theme is done. Opened PR https://github.com/tjscanlan/conduit/pull/11; no new work until it is merged. Then the v0.1.0 release cycle (`loop/release-v0-1`).

## 2026-11-04 — Cycle 11 (`loop/agent-readable-docs`, after PR #10 merged as 6e4c0b6): agent-readable docs, part 1
- Theme (user directive): make all docs first-class for agents to read.
- Did:
  - `docs/_data/docs.json` is the single docs manifest (sections, file, url, title, one-line description). It drives the site sidebar (Guides, SDKs, Project, and "For AI agents"), llms.txt, and checks; the old `nav:` in _config.yml is removed. No front matter was added to the doc files, so the GitHub view stays clean.
  - `scripts/build-agent-docs.py` (stdlib) runs after the Jekyll build and writes `_site/llms.txt` (llmstxt.org format: H1, summary blockquote, sections of `[title](raw .md URL): description`, an Optional section with llms-full and AGENTS.md), `_site/llms-full.txt` (all pages with source-URL markers, ~52 KB), and `_site/<page>.md` raw Markdown with front matter, Liquid, and generated-notes stripped. The overview's Markdown comes from the README (index.md is an HTML landing page). `--check` fails if a docs/*.md file is missing from the manifest, a listed file is missing, or a description is too short.
  - Layout: `<link rel="alternate" type="text/markdown">` per page, `rel="alternate"` to llms.txt, and a "View as Markdown" link.
  - **AGENTS.md** at the repo root: services and layout, exact commands, **invariants learned from real bugs** (identity from the cert only; validate before publishing; reserved metric names; Prometheus escaping; bound params; the time-binding truncation; idempotent migrations; CSP and template.URL; brand contrast; generated-code drift; dependency pinning), how to make a change, and where things are.
  - CI: the `generated` job runs the manifest check. The Pages build runs the generator, verifies every llms.txt link resolves to a built file and that no Liquid leaked, and the `.md`-link check was narrowed so the intentional `/conduit/*.md` links pass. The README and contributing guide point to llms.txt and AGENTS.md.
- Verified: ran the generator locally (manifest OK, 9 pages, no Liquid left, the overview built from the README correctly); YAML parses. The Pages build is pending in CI.
- Next: a machine-readable API reference generated from proto/*.proto (plus SDK APIs), then the PR.

## 2026-11-04 — **User directive for the NEXT cycle:** agent-first documentation
- The user asked (during cycle 10): "in the next loop, update all docs so that agents have first-class support to read them".
- Plan, for the cycle after `loop/metrics-logs-ingestion` merges (e.g. branch `loop/agent-readable-docs`):
  - Publish `/llms.txt` (llmstxt.org-style index with one-line summaries and links) and `/llms-full.txt` (all docs concatenated) on the Pages site.
  - Serve each page's raw Markdown next to its HTML (e.g. `/architecture.md`). That reverses `remove_originals`, or uses a generated copy without Liquid. Add `<link rel="alternate" type="text/markdown">`.
  - Add `AGENTS.md` at the repo root: how to build, test, and change the repo, its conventions, and where things live, for coding agents.
  - Consistent front matter on every doc (title, description, audience, last-reviewed), and self-contained sections (no "see above").
  - A machine-readable API reference generated from `proto/*.proto` (and the SDK APIs), plus a docs drift check in CI.

## 2026-11-04 — UI logs page
- Did: replaced the "not yet wired" placeholder with a real logs page.
  - `storage.QueryLogs(LogFilter)`: bounded by the window allowlist, at most 200 rows newest first, with bound parameters for agent, severities (from a fixed allowlist), case-insensitive search (`positionCaseInsensitiveUTF8`), and trace_id.
  - Filters: window, minimum severity (`warn` → {warn, error}), agent (≤256 valid UTF-8), text (≤200), trace (ID charset, ≤64). Invalid values are dropped.
  - Rows: time, severity badge (palette ramp, AA text), agent, the body in mono pre-wrap (escaped), and a trace link that deep-links `#span-<id>`.
  - htmx polling keeps the filter (a complete `template.URL`); a "showing newest 200" notice; filter-aware empty state. The trace detail links "Logs for this trace".
  - Added a `shortID` template func; a `{{slice .TraceID 0 8}}` draft would have panicked on short IDs (validID allows 1..64).
- Verified: handler tests (defaults, the severity ramp, filtered rows, form state, the trace notice, the empty state, the partial, polling URL, the detail link, XSS escaping, short IDs, and rejection of bad params). On the **real engine**: `severity IN ['warn','error']` on Enum8 (array literal, as clickhouse-go binds []string), case-insensitive search, and the full query all returned the right rows. Smoke checks the trace, severity, and text filters through the real driver (CI).
- **Stopping point:** the theme is done. Opened PR https://github.com/tjscanlan/conduit/pull/10; no new work until it is merged. Then the user's agent-first docs cycle.

## 2026-11-04 — Metrics pipeline + SDK Gauge/Counter
- Did:
  - `schema.MetricRequest.Validate`: Prometheus name and label-key regexes; **`conduit_*` and `__*` names reserved** (agents can't forge Conduit's own series); `__*` label keys rejected; at most 32 labels; values at most 1 KiB; finite values; counters ≥0; at most 1h in the future.
  - `telemetry.ProcessMetric` **always sets `agent` from the mTLS identity** (it overwrites any client label and clones the map rather than mutating it).
  - The gateway maps `pb.Metric` (unknown type enums are rejected) and publishes to a separate `METRICS` work-queue stream.
  - Collector: a third durable consumer (`collector-metrics`) writes VictoriaMetrics. **Failures retry**, because VM is the system of record for metrics (unlike the best-effort span-duration export).
  - SDKs: Go `Gauge`/`Counter`, Python `gauge`/`counter`, with gateway rejection reasons surfaced.
  - Smoke agents send gauges, and the smoke test asserts they're in VM with `agent=<cert CN>` (both SDKs).
  - Docs (SDK rows, architecture) updated; on the roadmap, "Metrics and logs ingestion" moved Now → Shipped.
- Verified: gateway test with 11 metrics (1 accepted; reserved, bad name, NaN, Inf, negative counter, bad and reserved label keys, too many labels, and unknown type rejected with matching reasons; a spoofed `agent` label overwritten); ProcessMetric (agent override, no mutation, defaults); collector metric dispositions; metric stream config; SDK tests against the mTLS fake (Go, and Python 10/10). Smoke runs in CI.
- Next: the UI logs page (it's a placeholder today), then PR.

## 2026-11-04 — **Security fix:** Prometheus label injection into VictoriaMetrics
- Found while starting the metrics work: `promLine` escaped only `"` in label values. The Prometheus text format requires escaping `\`, `"`, and newline. The `span` label is the agent-supplied span name (any characters), so:
  - a newline in a span name ended the sample and **injected arbitrary samples**, e.g. a forged `conduit_span_duration_ms{agent="victim"}`;
  - `\"` became `\\"`: the backslash was escaped, so the quote closed the value and broke out of it.
  Any authenticated agent could forge metrics attributed to other agents.
- Fix: escape all three with a `strings.Replacer`; sort label keys for stable output.
- Verified: unit tests cover plain, quote, backslash, newline injection, and backslash-quote breakout (one line per sample). **Against a real VictoriaMetrics v1.102.1** (throwaway container, 256 MB, no ports), importing the escaped payloads produced exactly 2 series with the payload text literal in the `span` label and no forged `victim` series.

## 2026-11-04 — SDK log APIs (Go + Python), logs in the smoke test
- Did:
  - Go: `client.Log(ctx, severity, body, attrs)` with `sdk.Severity*` consts. It reads the current `*Span` from ctx directly; `traceIDFromContext` would mint a new trace ID, which is wrong for a log outside a span. It sends via `IngestBatch` and returns the gateway's per-item reason on rejection.
  - Python: `client.log(severity, body, attributes=None)` with `SEVERITY_*` constants, links to the current span through contextvars, stringifies attribute values, and raises `ConduitError` with the gateway's reason.
  - Both smoke agents send a WARN log inside a span. The smoke test asserts the Go log row (agent_name from the cert CN, severity, body, attribute, span_id equal to the smoke.llm.call span) and the Python log row.
  - SDK docs gained API rows.
- Verified: SDK tests against a real mTLS gRPC fake gateway (Go and Python): no trace link outside a span, correct trace and span inside, severity, attributes, timestamp, and the rejection reason surfaced. Go vet and tests pass; pytest is 9/9. The smoke runs in CI.
- Next: metrics → VictoriaMetrics, then the UI logs page.

## 2026-11-04 — Cycle 10 (`loop/metrics-logs-ingestion`, after PR #9 merged as 53e7ea1): logs pipeline (server side)
- Theme: metrics and logs ingestion (roadmap Now). `IngestBatch` accepted both in the API and rejected them.
- Did, for logs end to end on the server:
  - `schema.LogRequest.Validate`: severity enum; body 1..32 KiB; no more than 1h in the future; optional trace and span IDs validated, with span_id requiring trace_id; attribute limits shared with spans via `validateAttributes`.
  - `telemetry.LogInput` and `ProcessLog` (defaults: unspecified severity, receive time).
  - The gateway maps `pb.LogRecord` (unknown severity enums pass through as invalid), stamps agent_name from the mTLS identity, and publishes per item, with errors reported per index. Rate limiting already charged logs per item.
  - NATS: a separate `LOGS` work-queue stream (`logs.ingest`), so backlogs are isolated; `ensureStream` is shared.
  - Collector: a second durable consumer (`collector-logs`) with the same ack/retry/term rules, writing ClickHouse via `InsertLog` (batch protocol, ns precision).
  - Migration 0003 creates the `logs` table. Retention reconcile is generalized to {spans, logs}, and the TTL regex now accepts any time column.
- Verified: tests cover gateway batch logs (1 accepted, 5 rejected with the right per-index reasons, published only to the log subject, identity not spoofable), ProcessLog defaults, collector log dispositions, the log stream config, the generalized TTL parse with the real engine string, and the migration resume test generalized to N migrations. On the real engine (clickhouse local 24.4), 0003 ran twice, MODIFY TTL on logs worked, and the normalized form matches the regex. Smoke now asserts the TTL on both tables and migrations 1..3 (CI).
- Next: SDK `Log` APIs (Go and Python), a smoke assertion that logs land, metrics → VictoriaMetrics, and the UI logs page.

## 2026-11-04 — Cycle 9 (`loop/clickhouse-migrations`, after PR #8 merged as 8013ddc): versioned schema migrations
- PR #8's first live deploy succeeded (pages run 41754108734, build and deploy green); https://tjscanlan.github.io/conduit/ serves every page, the CSS, and the logo with 200s.
- Theme: schema migrations, from the roadmap's Now lane (prompt access control still needs a human decision).
- Did:
  - `internal/storage/migrations/NNNN_name.sql`, embedded. 0001 is the spans DDL (initial 90-day TTL; retention is reconciled after), 0002 is the trace_summary MV, both byte-for-byte from the old constants, which are removed.
  - The migrator (stdlib) creates `schema_migrations` (ReplacingMergeTree(applied_at), read with FINAL), validates applied checksums (edits → refuse to start), refuses unknown future versions, applies pending versions in order one statement at a time (comments and trailing `;` stripped), and records each.
  - Loader rules: NNNN_name.sql, no duplicates, no gaps.
  - The collector migrates and then reconciles retention (`StoreOptions.ManageSchema`); a new `collector migrate` subcommand does it and exits. The UI no longer runs DDL.
  - Existing installs adopt automatically (0001 is IF NOT EXISTS, so it's a no-op that gets recorded).
- Verified: tests with a fake DB cover fresh-then-noop, resume after a mid-run failure, edited and unknown versions refused, loader validation, and idempotency enforced on the real files (one statement, IF [NOT] EXISTS). On the **real engine** (ClickHouse 24.4 `clickhouse local`, 512 MB cap, stdin because Colima doesn't share /private/tmp), both migrations applied twice without error, FINAL de-duplicated the double-recorded rows, and all objects exist with the expected engines. The smoke test now asserts `schema_migrations` = 1..N (CI). Caught a BSD/GNU `seq -s` difference in the smoke command (trailing comma) and switched to `paste -sd,`.
- **Stopping point:** opened PR https://github.com/tjscanlan/conduit/pull/9; no new work until it is merged.

## 2026-11-04 — Visual roadmap (single source) + Pages enabled
- Did:
  - `docs/_data/roadmap.json` is now the roadmap's single source of truth (18 shipped in 6 groups; Now 3, Next 6, Later 4; 3 open decisions).
  - Site: `_layouts/roadmap.html` renders a visual page: a stage timeline on a palette-gradient rail (Shipped check, Now red with a glow, Next and Later rings), color-coded Now/Next/Later lanes of cards with area tags, a Shipped panel with check bullets, and open-decision cards with option chips, the lean, and the raised-in PR link. Pills follow the contrast rules: white only on #ec0000 or ink; #ec5300, #ec9b00, and #ecca00 carry ink.
  - `scripts/gen-roadmap.py` (stdlib) generates the GitHub-readable `docs/roadmap.md` from the JSON; `make roadmap`. CI's `generated` job and the Pages build both fail if they drift.
  - `optional_front_matter.remove_originals` so raw `.md` sources aren't published. `contributing.md` got front matter, since jekyll-optional-front-matter skips CONTRIBUTING (and README/LICENSE) without it (caught by the build check).
  - **Enabled GitHub Pages** (build_type=workflow) at https://tjscanlan.github.io/conduit/; it first deploys when this branch merges to main. README has a docs badge and link.
- Verified: both workflows are green; I downloaded the built Pages artifact and screenshotted the home, deployment, and roadmap pages (the brand renders correctly and links are rewritten to `/conduit/*.html` including anchors). **The live deploy can't be verified until merge.**
- **Stopping point:** opened PR https://github.com/tjscanlan/conduit/pull/8; no new work until it is merged. After merge: verify the first live Pages deploy.

## 2026-11-04 — Cycle 8 (`loop/docs-site-roadmap`, after PR #7 merged as fa8a13f): GitHub Pages site scaffold
- Theme (user directive): a GitHub Pages documentation site plus a visual product roadmap, using the Conduit brand.
- Did:
  - Jekyll site with `docs/` as the source, built by GitHub's own `jekyll-build-pages` action, so the repo gains no Ruby or npm dependencies. Only plugins Pages supports natively: relative-links (`.md` links work on GitHub and on the site), optional-front-matter, titles-from-headings, seo-tag.
  - Branded layout: an ink header with a gradient rule, the logo and "Conduit Docs", a sticky sidebar nav, an edit-on-GitHub link, and a footer. site.css reuses the app's tokens and contrast rules.
  - Home page (docs/index.md): a hero with the logo and CTAs, six feature cards with palette accents, and a docs grid.
  - Mermaid renders client-side from jsDelivr, **pinned to 11.17.2 with an SRI sha384**, loaded only on pages that have diagrams, with a brand-themed config and securityLevel strict.
  - `.github/workflows/pages.yml`: every push builds; the deploy job is gated to main (pages:write and id-token:write only there). Checks: the logo and favicon copies match the app's (`cmp`), every page exists, and no unconverted `.md` links remain. Actions are pinned by SHA.
  - The brand.md logo now uses a path that works on both GitHub and the site.
- Next: the visual roadmap page (data-driven, branded), inspect the built artifact, enable Pages (Source: GitHub Actions), then the PR.

## 2026-11-04 — **User directive for the NEXT cycle:** GitHub Pages docs site + visual roadmap
- The user asked (during cycle 7): "in the next loop, create a github-pages site for the documentation and a visual product roadmap using the brand identity created in the earlier loop".
- Brand to apply: the logo mark (ui/static/img/logo.svg) and favicon; the palette `#ec0000` `#ec2400` `#ec5300` `#ec9b00` `#ecca00` plus white, with ink `#1c1917`; the contrast rules in docs/brand.md (only #ec0000 as text on white; #ec9b00 and #ecca00 take ink, never white text).
- Plan: finish cycle 7 (configurable retention) → PR → after it merges, start a branch like `loop/docs-site-roadmap` that builds the GitHub Pages site from docs/ (and a deploy workflow) plus a visual roadmap page.

## 2026-11-04 — Configurable retention; fix ClickHouse TLS downgrade
- Did:
  - `SPAN_RETENTION_DAYS` (collector, default 90, range 1–3650) sets the spans TTL in the DDL.
  - The collector (`StoreOptions.ManageRetention`) reconciles existing tables: it reads `system.tables.engine_full` and runs `ALTER TABLE spans MODIFY TTL` only when the TTL differs. The UI only creates tables if missing.
  - The days value is a validated int (never user text). A non-numeric env var maps to -1, so the range check rejects it rather than silently defaulting.
- **Security bug fixed:** `NewClickHouseStore` did `opts.TLS = nil // handled externally via mTLS at network layer`, so a DSN with `secure=true` was **silently downgraded to plaintext** (credentials included); the "network-layer mTLS" didn't exist. TLS now follows the DSN, with a regression test.
- Verified: ran `clickhouse local` (24.4, 512 MB cap, the demo stack unaffected) to capture the real engine_full normalization (`TTL toDateTime(start_time) + toIntervalDay(90)`) before and after MODIFY TTL; the tests use those exact strings. Also tested DDL rendering, retention range checks before connecting, TLS kept for `secure=true` and absent otherwise, and env parsing. CI smoke now runs with `SPAN_RETENTION_DAYS=30`, so the ALTER path runs on a real ClickHouse, and asserts `toIntervalDay(30)`.
- **Stopping point:** both data-governance items are done. Opened PR https://github.com/tjscanlan/conduit/pull/7; no new work until it is merged. Then the user's GitHub Pages + visual roadmap cycle.

## 2026-11-04 — Cycle 7 (`loop/payload-retention-controls`, after PR #6 merged as c2ddcbf): STORE_LLM_PAYLOADS
- Theme: data-governance controls from the roadmap's Now list that need no human decision: a switch to stop storing payloads, then configurable retention.
- Did:
  - Gateway `STORE_LLM_PAYLOADS` (default true, so behavior is unchanged). When false, prompts and completions are stripped in `ingestOne` **before validation and before publishing**, so they never reach NATS, logs, or ClickHouse; model, tokens, and cost are kept. Stripping first means an oversized prompt that will be discarded doesn't get the span rejected.
  - Bool parsing accepts true/false, 1/0, yes/no, on/off. **An unrecognized value stops the gateway at startup** (`validateConfig`), so a typo like `flase` can't silently keep storing prompts.
  - The handler takes a `spanPublisher` interface (testable without NATS).
  - Exposed in compose (`${STORE_LLM_PAYLOADS:-true}`) and the k8s configmap. Docs: architecture "Sensitive data", deployment config, a security threat-model row, and the roadmap item moved to Shipped.
- Verified: tests capture the exact bytes published to NATS. With the switch off, neither payload string appears and model, tokens, and cost are kept; an oversized prompt is rejected with storage on and accepted with it off; payloads are kept by default; config parsing covers all accepted spellings and typos. Gateway vet and tests pass.
- Next: configurable retention (the 90-day TTL is hard-coded in the DDL; existing tables need `ALTER TABLE … MODIFY TTL`).

## 2026-11-03 — Formal docs and product roadmap (user directive, cycle 6 continued)
- Did:
  - README rewritten: logo header, CI/Go/license badges, highlights, an accurate Mermaid architecture diagram, a quick start that works (cert generation, users file, CLICKHOUSE_PASSWORD, `up --wait`, first trace via the smoke agent), a documentation index, project status, and the license.
  - docs/architecture.md rewritten: a components table, a Mermaid data-flow sequence, delivery and failure behavior, the storage schema (including end_time, ingested_at, trace_summary semantics, the hard-coded 90-day TTL), UI query bounds, a sensitive-data section, operational endpoints, and design decisions.
  - New docs/roadmap.md: Shipped, Now, Next, Later, and a table of the open decisions.
  - New docs/brand.md: logo usage, the palette with roles, neutrals, and WCAG contrast rules from computed ratios.
  - docs/contributing.md (was empty): prerequisites, layout, required checks matching the CI jobs, codegen, conventions (deps, tests, bound params, the time-binding pitfall, CSP, template.URL, brand rules), PR process, automated-loop contributions, private security reporting.
  - docs/security.md gained a threat model table, with operator responsibilities listed as out of scope.
- **Inaccuracies corrected from the old docs:**
  - "Wire format OTLP": it's Conduit's own protobuf API; only collector → Tempo uses OTLP.
  - The diagram showed the UI reading Tempo and VictoriaMetrics; it reads only ClickHouse.
  - "NATS in-process": it's remote only.
  - **`OTEL_INCLUDE_PAYLOADS=true` doesn't exist.** Prompts are stored whenever an SDK records them. The docs now say so plainly, and the roadmap lists a server-side switch.
  - The stale quick start (gen-proto, missing CLICKHOUSE_PASSWORD and users file).
  - "Configurable TTL": it's hard-coded.
- Verified: a script checked every relative link and anchor in the README and docs (0 broken), and the doc claims were checked against the code (gateway publish-error path, exporter writes of prompts and completions, IngestBatch rejecting metrics and logs).
- **Stopping point:** opened PR https://github.com/tjscanlan/conduit/pull/6 (branding plus docs plus roadmap); no new work until it is merged.

## 2026-11-03 — Cycle 6 (`loop/docs-roadmap-branding`, after PR #5 merged as cf4ccd1): branding, iteration 1
- Did: SVG logo mark (five concentric arcs in the palette, red outside to yellow inside, forming a "C" open toward the flow) and favicon; full app.css rewrite around design tokens. Light white UI, warm near-black ink text, ink nav bar with a 3px palette-gradient rule, sans UI with mono for IDs, the logo plus wordmark in the nav, and a login card with the logo, tagline, and gradient top. metrics.js canvas colors updated.
- Contrast rules (computed): only #ec0000 passes AA as text on white (4.6:1), so it's used for links, buttons, and errors. #ec9b00 and #ecca00 take ink, never white. Error text on the red tint uses #b80000 (6.3:1).
- Verified: go test; CSP template test (no inline styles). **Not yet viewed in a browser.**
- Visual check (follow-up): rendered the real templates with sample data through a throwaway test harness (kept in the scratchpad, never committed) and screenshotted them with WebKit (qlmanage). It found two layout bugs, both now fixed:
  - Global `nav {}` rules styled the trace-list pager as a second dark nav bar, so they're now scoped to `.topnav`.
  - The span row grid had 5 tracks but 6 children on LLM spans (the hidden `#` link and the LLM tag each took a cell), so the waterfall wrapped. The row now has 4 stable tracks: title+link | status+tags | timing | bar.
  - The list, detail, and login pages now look right.
- Remaining in this cycle: formal docs (README overhaul with logo, docs index, architecture refresh, contributing.md), docs/roadmap.md, a browser check of the new look, then PR #6.

## 2026-11-03 — **User directive for the NEXT cycle:** formal docs, product roadmap, professional branding
- The user asked (while cycle 5 was in progress): "in the next loop, please focus on formalizing the documentation, adding a product roadmap, and making the logo and graphics of the ui look more like a professional piece of software."
- **Brand palette (user-specified), plus white:** `#ec0000` (236,0,0) · `#ec2400` (236,36,0) · `#ec5300` (236,83,0) · `#ec9b00` (236,155,0) · `#ecca00` (236,202,0).
- Plan: finish `loop/service-health-checks` → PR #5 → after it merges, start `loop/docs-roadmap-branding` (or a similar descriptive name) with exactly this theme.

## 2026-11-03 — Cycle 5 (`loop/service-health-checks`, after PR #4 merged as e82857d): health endpoints, probes, restart policies
- Theme: service health. The gateway and collector had no health endpoints, so k8s couldn't detect a wedged pod and compose couldn't order startup. The compose infra had no restart policy (after a VM restart it stayed down while the apps crash-looped).
- Did:
  - `internal/health` (stdlib only): `/livez` and `/readyz` on a plain-HTTP `HEALTH_ADDR` (default :8081). It's a separate port because the gateway's gRPC port requires mTLS client certs that probes can't present. Each check has a 2s timeout. `/readyz` reports only ok or unavailable per check, and errors are logged, never returned (no DSN or host leaks).
  - Checks: gateway checks NATS; collector checks NATS and ClickHouse (VictoriaMetrics and Tempo are best-effort, so they don't gate readiness); the UI's `/readyz` on its main port checks ClickHouse (unauthenticated, like /healthz).
  - `Ready()` methods on NATSPublisher, Collector, and ClickHouseStore (ping).
  - Every binary has a `healthcheck` subcommand that GETs its own /readyz and exits 0 or 1, since scratch images have no curl or wget.
  - compose: `restart: unless-stopped` on all infra, and healthchecks (`/app healthcheck`) on gateway, collector, and ui, so CI's `up --wait` now waits for real readiness.
  - k8s: probes on 8081 for gateway and collector; the ui readiness probe now uses /readyz.
  - docs/deployment.md gained a "Health checks" section and HEALTH_ADDR.
- Verified: tests for health (livez ignores checks, readyz ok and failing, no error leak, hung checks bounded, Probe exit codes and address normalization) and the UI (/readyz unauthenticated in login mode, 503 when ClickHouse is down). The smoke test runs `/app healthcheck` in each service (CI). The YAML parses.
- **Stopping point:** opened PR https://github.com/tjscanlan/conduit/pull/5; no new work until it is merged. Then the user's docs, roadmap, and branding cycle.

## 2026-11-03 — Trace list: agent suggestions
- Did: `storage.QueryAgents(window, limit)` (DISTINCT non-empty agent_name in the window, sorted, at most 200) feeds a `<datalist>` on the agent filter input, so there's no JS and free text still works. It's skipped for htmx partial refreshes (rows only), and a failed query only drops the suggestions (logged at warn); the page still renders.
- Verified: handler tests (both agents suggested, no agent query on htmx partials, page renders when suggestions fail). The smoke test checks that the real list page suggests the smoke agent (runs in CI).
- **Stopping point:** the trace-list theme is done (one row per trace, filters, paging, suggestions). Opened PR https://github.com/tjscanlan/conduit/pull/4; no new work until it is merged.

## 2026-11-03 — Trace list: keyset paging
- Did: an "Older →" link pages past the 100-trace limit using a keyset cursor `before=<startedUnixNano>.<traceID>`.
  - Query: `HAVING (started, trace_id) < (fromUnixTimestamp64Nano(toInt64(?)), ?)` with `ORDER BY started DESC, trace_id DESC`. Stable as new traces arrive; the trace_id tiebreak means no gaps or repeats at equal start times. The cursor time is bound as integer ns, avoiding second truncation.
  - Fetches limit+1 to detect another page. "← Newest" returns to page 1. Older pages don't auto-refresh. The cursor is validated: positive int64 ns plus an ID charset of at most 64 chars.
- **Bug caught by the new test:** the first version interpolated a pre-encoded query into `href="/traces?{{.}}"`, and html/template URL-escapes query-position values, so `=`/`&` became `%3d`/`%26` and Older silently reloaded page 1. Pager links are now complete `template.URL`s built from `url.Values.Encode()`; the test asserts they aren't double-encoded. (hx-get didn't have this problem: html/template doesn't treat it as a URL attribute.)
- Verified: handler tests (exactly 100 then 30 rows across 130 traces, no overlap or gap at the boundary, the right links on each page, no polling on older pages, cursor round trip, malformed cursors rejected and dropped from the filter). The smoke test now checks the cursor against real ClickHouse in CI: the page older than the Python trace contains the earlier Go trace and not the cursor trace. Not run locally, because the user's demo stack occupies the local resources.
- Next candidates: an agent dropdown from recent agents; `restart: unless-stopped` for compose infra; then a PR for this theme.

## 2026-11-03 — Cycle 4 (`loop/trace-list-filters`, after PR #3 merged as f5e7927): one row per trace + filters
- Theme: the traces list. `/traces` listed the latest 100 *spans* (one chatty trace filled the page), with no filtering.
- Did:
  - `storage.QueryTraces(TraceFilter)`: groups spans by trace_id, returning the root span name (`argMin(span_name, (parent_id != '', start_time))`), start, end, span and error counts, agents, tokens, and cost, newest first.
  - The scan is always bounded by a window (1h, 24h, or 7d allowlist), passed as integer seconds to `now64() - toIntervalSecond(?)`, which avoids the time.Time second-precision binding.
  - The agent filter selects whole traces containing the agent (an `IN` subquery), so totals still cover every span. Errors-only uses `HAVING`.
  - UI: a GET filter form (window select, agent search, errors-only checkbox, Clear) with no JS. htmx polling keeps the filter (`hx-get="/traces?{{.Filter.Query}}"`). New columns: Started, Trace (root name links to detail, plus ID), Agents, Spans, Errors, Duration, Tokens, Cost. The empty state is aware of filters.
  - Params are validated: window is allowlisted; agent must be ≤256 bytes of valid UTF-8 (still a bound parameter); `errors` must be exactly `1`.
  - Shared `parseTemplates()` with a FuncMap (`durationMs`). Removed `QueryRecentSpans`.
- Verified: ran the exact SQL on live ClickHouse before wiring it (correct root names and counts; errors+agent filter keeps all 4 smoke traces; unknown agent returns 0 rows). Handler tests: one row per trace, links named by root, default filter, polling URL, filtered form state, the errors-only and agent semantics, the empty state, and rejection of bad params. The smoke test now checks the list and its filters against the real stack (CI).
- Environment note: the user started a `conduit-demo` compose project from this checkout (`.local/demo`, self-ignored), which held :8428. Two stacks don't fit Colima's 2 GiB (tempo exited 137), and my dev stack's infra had stopped. I stopped **only my** `docker` project (kept its volumes) and left the demo alone. Live checks for this cycle run in CI.
- Next candidates: paging (an "older" cursor); `restart: unless-stopped` for compose infra services (after a VM restart they stayed down while the apps crash-looped); agent dropdown populated from recent agents.

## 2026-11-03 — Trace detail: bound huge traces; span deep links
- Did:
  - `QueryTrace` takes a limit. The detail page loads at most 2000 spans (fetching 2001 to detect truncation) and shows a notice that the totals cover only those spans. Before, a runaway agent could make the UI fetch and render an unbounded trace.
  - Each span row has `id="span-<spanID>"` and a hover `#` link; `/traces/<id>#span-<spanID>` opens, highlights, and scrolls to that span (traces.js, which also handles `hashchange`).
  - Removed traces.js's dead `highlightCurrent`, which targeted the old flat table and set an inline style.
- Verified: handler tests for exactly 2000 rendered spans plus the notice, no notice on small traces, and the anchors and links being present. traces.js parses (JavaScriptCore; node isn't installed). The smoke test passes. **Deep-link behavior isn't browser-tested.**
- **Stopping point:** the trace-detail theme is done. Opened PR https://github.com/tjscanlan/conduit/pull/3; no new work until it is merged. Next themes: the traces list (one row per trace, filter by agent or status), and LLM payload display (needs an access-control decision).

## 2026-11-03 — Cycle 3 (`loop/trace-detail-view`, after PR #2 merged as cba594e): trace detail page; fix second-truncated timestamps
- Theme: trace detail view. Users couldn't see attributes, error.message, LLM model, tokens, or cost, which is the core of an LLM observability tool.
- Did:
  - `cmd/ui/traceview.go` `buildTraceView`: spans in depth-first tree order (children under parents by start time; spans with missing parents become roots; cycles, self-parenting, and duplicate IDs are each shown exactly once), depth capped at 10, waterfall geometry, totals (duration, spans, errors, tokens, cost, agents), and preformatted LLM fields.
  - New `trace` template: summary bar; one `<details>` row per span with name, status badge, LLM tag, offset and duration, and an SVG waterfall bar. Expanding a row shows error.message, IDs, agent, LLM model/tokens/cost, and a sorted attribute table (excluding promoted keys).
  - The CSP holds: indentation uses `depth-N` classes and bars use SVG x/width attributes, with no inline styles.
  - Prompts and completions are **not shown** (the proto reserves "payload-level access control", and the query doesn't select them). **Decision for the human.** The traces list template lost its detail branch.
- **Data bug found while checking the page:** every ClickHouse start_time and end_time was **truncated to whole seconds**. `InsertSpan` used `Exec` with `?` placeholders, and clickhouse-go's positional binding formats time.Time at second precision. duration_ms looked right only because the exporter computes it. Switched to `PrepareBatch`/`Append` (native DateTime64(9)). The smoke test now asserts sub-second precision, and it fails against the old collector (0 ms) and passes against the new one (21 ms). **Existing rows stay truncated**; that can't be recovered.
- Verified: tests for tree order, totals and geometry (including instant spans at the end), cycles and duplicates, the depth cap, and escaping of hostile attribute and error values. The handler test checks the detail page. Live: the full smoke test passes, and the page for a real trace shows 111 ms total with the child bar at 189/1000.
- Next candidates: list page improvements (one row per trace rather than per span, filtering by agent or status); show prompts/completions behind an explicit access control (needs a decision); attribute search.

## 2026-11-03 — Loop: descriptive branch names (requested by user)
- Decision (user): each loop branch must have a useful name after `loop/` that describes the feature being iterated on.
- Did: added "Branch names" to `.claude/loop.md` step 0: `loop/<feature-name>` in kebab-case, 2–5 words, naming the outcome, never dates, counters, or `auto`/`wip`/`misc`. Pick the theme before creating the branch. Theme drift is a stopping point for a PR, and the PR title matches the theme. The orient step now refers to the current `loop/<feature-name>` branch.
- Note: the current branch `loop/20261103-1353` keeps its name, since it's PR #2's head and renaming it would close the PR. The rule applies from the next cycle.

## 2026-11-03 — Python SDK in the smoke test; docs/sdk-python.md
- Did:
  - `scripts/smoke-test.sh` now also sends a trace with the **Python** SDK (`sdk/python/examples/smoke_agent.py`) through the real gateway and asserts its ClickHouse rows: agent_name from the cert CN, ok and error statuses, exception → error.message, LLM model, 16-hex IDs, and the parent link. It fails if the SDK isn't importable, unless SMOKE_SKIP_PYTHON=1. The CI smoke job installs the SDK first.
  - New API: `client.span(name, raise_errors=True)` for strict delivery in context-manager form. It never masks the block's own exception. Found when the smoke agent needed it; tested.
  - Wrote `docs/sdk-python.md` (certs, install, instrumentation, an API table with gateway limits, delivery-error semantics, caveats, development), plus pointer READMEs for both SDKs (they were empty).
- Verified: the full smoke test passes locally, including the Python trace. pytest is 8/8. The documented API was checked against the real signatures.
- **Stopping point: opened PR https://github.com/tjscanlan/conduit/pull/2. No new work until it is merged.**

## 2026-11-03 — Python SDK: from stub to working package
- Did: the Python SDK was in the state the Go SDK was in this morning, plus more:
  - `_flush` was a no-op "Stub", so nothing was ever sent.
  - The documented `agent_observability` package didn't exist; hatch pointed at `sdk/python` from inside it, and the modules used relative imports from a dir named `python`.
  - The current span lived on the Client instance, so the "thread-safe" claim was false and concurrent threads or tasks cross-linked parents.
  - There was no status support, and exceptions weren't recorded.
  - The pyproject said MIT while the repo is Apache-2.0.
- Fixes:
  - Package: `sdk/python/src/conduit_sdk`.
  - Delivery: real `IngestSpan` over mTLS (trusts only TLS_CA).
  - Context: tracked in `contextvars`, safe across threads and asyncio tasks.
  - Status: `set_ok`/`set_error` (None is a no-op; the message is UTF-8-truncated to 4096 bytes); an exception in `with client.span()` sets error and re-raises.
  - Delivery errors: explicit `end()` raises `ConduitError`, and the `with` block only logs, so instrumentation can't break the agent. `end()` is idempotent.
  - Env: `CONDUIT_GATEWAY_ADDR` (OTEL_GATEWAY_ADDR accepted).
  - License: Apache-2.0.
- Generated stubs: `scripts/gen-proto-python.sh` (pinned grpcio-tools 1.85.1, via uv or pip; rewrites protoc's top-level imports to package-relative) and **committed**, consistent with the Go decision. `make proto` runs both generators, and the CI drift check now covers the Python stubs. Deps are pinned to the stubs' floors (grpcio>=1.85.1, protobuf>=7.35.1,<8). Ignore Python caches repo-wide.
- Tests (pytest, an in-process gRPC server requiring client certs, certs minted with `cryptography`, which is dev-only): delivery and parent linking, W3C ID sizes, LLM fields, exception → error, set_error(None), truncation, rejection raising vs. context-manager logging, idempotent end, untrusted CA refused, thread isolation, and asyncio task isolation. New CI job `python-sdk` on Python 3.10 and 3.14 (setup-python pinned by SHA).
- Verified: 7/7 pytest pass on 3.12 locally, Python codegen is deterministic, and Go is unaffected.
- Next candidates: write docs/sdk-python.md; make the smoke test also send a Python trace through the real gateway; contributing.md. Then open PR #2 (this cycle will have the smoke job, the UI detail fix, the cert fixes, and the Python SDK).

## 2026-11-03 — Fix smoke CI: stale tracked certs + gen-certs reissue bug
- CI run 41698657967 failed in `compose smoke test` at gen-certs: "certs/agent-example.crt already exists".
  - Cause 1: `certs/{ca,server,agent-example}.crt` had been tracked since the initial "mvp-ish code" commit, before `certs/` was gitignored. They're **public certificates only**. I scanned all history: no private key was ever committed (the only "PRIVATE KEY" matches are PEM labels in sdk_test.go). They're useless without their keys, so I untracked them; local copies stay.
  - Cause 2 (my bug from 388d75b): a full `gen-certs.sh` run creates a new CA, then refused to reissue agent-example because a cert already existed. That cert was signed by the old CA, so it was invalid. The full run now always reissues the example agent cert.
- Next CI run (41699093345): the whole pipeline worked in CI for the first time. The only mismatch was the expected agent name: the rewritten gen-certs uses the file name as the CN (`agent-example`), and the old script used `example-agent`. The smoke test now reads the expected CN from the cert (handling both LibreSSL and OpenSSL 3 subject formats), which makes the cert-to-agent_name check stricter.
- Verified: reproduced the CI case in scratch (stale public certs, no keys). The full run now exits 0 and `openssl verify` passes for both server and agent certs. A rerun with a CA present still refuses, and agent mode still works.

## 2026-11-03 — Cycle 2 begins (PR #1 merged as e914c06). Compose smoke test in CI; trace detail page fixed
- PR gate: PR #1 was merged at 18:50 UTC (merge commit, history kept). Started branch `loop/20261103-1353` from origin/main.
- Did: automated the manual smoke test so deployment regressions like the four fixed in a5600cd fail CI.
  - `scripts/smoke-test.sh`: sends a trace with the SDK and asserts ClickHouse rows (agent_name from cert CN, statuses, error.message, LLM fields, span ID length, parent), trace_summary totals, VictoriaMetrics series, the Tempo trace with OK and ERROR statuses, and the UI (healthz, login redirect, 401, cross-site 403, login, trace detail, CSP). Retries cover eventual consistency.
  - `sdk/go/examples/smoke-agent` sends the trace. New SDK API: `Span.TraceID()` and `Span.SpanID()` (log correlation); tested.
  - CI job `smoke`: random masked passwords, gen-certs, `compose up --build --wait`, the script, logs on failure, and `down -v` always.
  - **The smoke script found a UI bug right away:** `/traces/{id}` passed `Spans`, but the template iterates `Rows`, so the detail page always showed "No spans yet". It also polled `hx-get="/traces"`, which would swap in the global list. Fixed: it passes Rows, shows a "Trace <id>" heading and a back link, doesn't poll, and returns 404 for unknown traces. The UI handlers now take a `spanQuerier` interface; added tests for the detail, list, and 404 cases. (My manual check before PR #1 only looked at the HTTP 200.)
- Verified: the smoke test passes against the local compose stack after rebuilding the UI. go vet and test -race pass for root and sdk. CI pending.
- Next candidates: Python SDK tests plus a CI job (it's untested; the smoke harness could send a Python trace too); contributing.md and sdk-python.md; UI trace detail could show attributes and LLM fields (the SpanRow query doesn't select prompt or completion, which is probably right by default).

## 2026-11-03 — Compose smoke test (requested by user): 4 bugs fixed; full pipeline verified
- Ran `colima start` (already running: aarch64, 2 CPU, 2 GiB) and installed the docker compose plugin (brew). Then `docker compose up`. Fixed what it found:
  - **Dockerfile:** every image compiled all three binaries (9 parallel compiles OOM-killed the 2 GiB VM), and `GOARCH=amd64` was hard-coded (exec format error on arm64). It now builds only `${BINARY}`, with `GOARCH="${TARGETARCH:-$(go env GOARCH)}"` and `-trimpath`. Note: a first attempt using `${TARGETARCH:+GOARCH=…}` only worked because it expanded to nothing; POSIX sh doesn't treat expanded text as an assignment. Fixed before committing.
  - **ClickHouse DDL never worked anywhere:** `TTL start_time + …` on DateTime64 is rejected (code 450), so the collector and UI could never start. It's now `toDateTime(start_time)`. The `trace_summary` MV also failed (DateTime64 subtraction), and its design was wrong (plain min/max/sum columns in an AggregatingMergeTree keep an arbitrary row on merge). Rewrote it with `-SimpleState` columns, adding span_count and computing duration at query time. Verified the merge across separate inserts with OPTIMIZE FINAL.
  - **compose:** startup race (the collector and UI exited before ClickHouse was ready, with no restart). Added a ClickHouse healthcheck (`/ping`), `depends_on: service_healthy`, and `restart: unless-stopped`. An unset `CLICKHOUSE_PASSWORD` silently became blank; it's now `${CLICKHOUSE_PASSWORD:?…}`. Removed the obsolete `version:` key.
- Verified end to end: Go SDK → gateway (mTLS, agent-example cert) → NATS → collector → ClickHouse, VictoriaMetrics, and Tempo. ClickHouse has both spans (agent_name came from the cert CN despite an attempted spoof, ok and error status, error.message, LLM fields, 16-hex span IDs, parent link) and trace_summary has 2 spans, 1 error, 12 tokens, $0.0042. VictoriaMetrics has duration gauges, and Tempo has both spans with STATUS_CODE_OK and STATUS_CODE_ERROR and the parent link. UI: redirect to login, 401 on a bad password, 403 on a cross-site login, cookie login, traces page with both spans and badges, detail 200, logout. /healthz and the CSP header are OK. Zero collector errors.
- Next candidates (after merge): a CI job that runs this compose smoke test automatically; Python SDK tests and CI; contributing.md and sdk-python.md.

## 2026-11-03 — W3C/OTLP-sized IDs, so Tempo accepts spans
- Did: both SDKs generated 32-hex (16-byte) span IDs, but OTLP and W3C require 8 bytes, so with the Tempo endpoint now fixed, Tempo would still have rejected every span. The Go SDK now uses `newTraceID` (16 bytes) and `newSpanID` (8 bytes). The Python SDK uses `secrets.token_hex(16)` and `secrets.token_hex(8)` instead of uuid4 hex. The exporter skips Tempo for IDs that aren't OTLP-shaped (exact-length hex, not all zero), since the gateway also accepts opaque IDs like UUIDs with dashes. ClickHouse and metrics are still written for those spans.
- Verified: go vet and go test -race PASS for root and sdk (new: otlpIDs table, Tempo skipped for UUID IDs, SDK IDs asserted 32/16 hex). Python SDK: syntax-checked only; it can't be imported here because grpc isn't installed, and it has no tests.
- Next candidates (for after the PR merges): Python SDK tests plus a CI job; empty contributing.md and sdk-python.md; UI trace-detail view (attributes, error.message); Dockerfile GOARCH; a compose smoke test by a human.
- **Stopping point: opened PR https://github.com/tjscanlan/conduit/pull/1 (loop/auto → main, user request). No new work until it is merged.**

## 2026-11-03 — Loop now delivers work by PR and waits for merge (requested by user)
- Decision (user): stop after the next iteration and open a PR. From now on, open PRs at reasonable stopping points, and don't continue looping until the PR is approved and merged to main.
- Did: `.claude/loop.md` gained "0. PR gate" (no new work while a loop PR is open; address CI failures and review comments on it; after a merge, start a fresh `loop/<date>-<time>` branch from origin/main; if a PR is closed unmerged, stop and ask), "6. Open a PR at a reasonable stopping point", and "Waiting for merge" (a background watcher plus a long fallback, no timer iterations). The push rule is now limited to the loop's own `loop/*` branch, and it never merges.
- Note: the PR is authored by the repo owner's account and GitHub doesn't allow self-approval, so the gate is **merged to main with no outstanding changes-requested review**.
- **Pending:** the next iteration does one more task, then opens the first PR (loop/auto → main).

## 2026-11-03 — Deployment fixes (k8s mTLS chain, Tempo) + docs/deployment.md
- Did: wrote `docs/deployment.md` (components and ports, the compose quickstart, k8s certs, secrets, and apply steps, upgrade notes, a config reference). Writing it turned up real deployment bugs:
  - **k8s mTLS couldn't work:** the `selfSigned` ClusterIssuer made the gateway cert its own CA, so agent certs couldn't chain to it. Replaced it with the standard bootstrap selfSigned issuer, then a root CA Certificate (in cert-manager's namespace), then the `conduit-ca-issuer` CA ClusterIssuer. The gateway cert has `usages: server auth`, and there's an example agent cert with `client auth`.
  - **k8s gateway couldn't start:** cert-manager writes `tls.crt`, `tls.key`, and `ca.crt`, but the gateway reads `server.crt` and `server.key`. The volume now maps the keys onto those paths.
  - **Every Tempo write failed:** the collector POSTed to `:4317/otlp/v1/traces`. 4317 is OTLP *gRPC*, and `/otlp/...` is the Grafana Cloud gateway path. It now posts to `/v1/traces` on 4318 (the collector default, compose, and the k8s configmap). It was never noticed because Tempo errors are logged as best-effort.
  - **compose Tempo couldn't start:** `./tempo.yaml` was mounted but didn't exist. Added a minimal single-node config.
- Verified: go vet and go test -race PASS (a new test checks the POST path and content type). The k8s and compose YAML parse. The doc's hash-password pipeline produces a valid users line, and the compose cert paths match gen-certs output. **Not verified against a real cluster, cert-manager, or Tempo**, and the docker compose stack wasn't started (loop rule).
- Next candidates: **OTLP ID lengths.** The SDK makes 16-byte span IDs (OTLP and W3C require 8), and the schema accepts non-hex IDs like UUIDs with dashes, so Tempo will reject them. Generate 8-byte span IDs in the SDK, and in the exporter skip or normalize IDs that aren't OTLP-shaped for Tempo only. Also contributing.md and sdk-python.md are still empty.
- Blockers / needs human: someone should run `docker compose up` once to smoke-test the stack. nats-server test-dependency decision (still open).

## 2026-11-03 — Go SDK docs; gen-certs.sh agent mode (and stop clobbering the CA)
- Did: wrote `docs/sdk-go.md`: issuing a certificate, consuming the module (go.work for now, since it's unpublished and uses a replace), instrumentation, an API table with the real gateway limits, rate limits, and caveats (synchronous End, no retries, identity always from the cert CN, a stale EstimateCostUSD table). Added `sdk/go/example_test.go`, which mirrors the doc snippet so it keeps compiling.
  - `scripts/gen-certs.sh` printed `CN=my-agent ./scripts/gen-certs.sh agent`, but there was no agent mode, so following that advice **regenerated the CA and invalidated every issued cert**. Added `gen-certs.sh agent NAME` (validated name, client-auth extended key usage, refuses to overwrite without FORCE=1). Plain `gen-certs.sh` now refuses when a CA exists (FORCE=1 overrides). Keys are written 0600, the server cert gets serverAuth, and CERTS_DIR can be overridden.
- Decided **not** to add nats-server/v2 as a test dependency for a gateway → NATS → collector integration test. It's a large module tree, at odds with the project's no-supply-chain-surprises pitch. The handle(), exporter, and processor unit tests cover the logic. **Needs a human call if you want it.**
- Verified: ran the script in scratch (full run, refusal on rerun, agent issuance, duplicate and `../evil` names rejected); `openssl verify` chains all certs; a real Go TLS 1.3 mTLS handshake with the OpenSSL-made certs gives peer CN=research-bot. sdk vet and test PASS (the example compiles). Real certs/ untouched.
- Next candidates: **the k8s dev ClusterIssuer is `selfSigned`, so the gateway cert is its own CA and agent certs can never chain to it.** Use the standard selfSigned, then CA Certificate, then CA Issuer pattern, and write docs/deployment.md alongside. Also the empty contributing.md and sdk-python.md (check the Python SDK actually works first).
- Blockers / needs human: `gh auth login`; the nats-server test-dependency decision above.

## 2026-11-03 — Rate limit: batches could bypass the per-agent limit
- Did: the limiter charged 1 token per RPC, so one `IngestBatch` carrying thousands of spans cost the same as one span. Within gRPC's 4 MB limit that's about 40k small spans per call, so an agent allowed 100/s could send millions per second.
  - Now charges one token per telemetry item (spans + metrics + logs).
  - Caps batches at 1000 items.
  - Rejects with InvalidArgument any batch larger than the bucket (it could never succeed), and keeps ResourceExhausted for retryable rejections.
  - `RATE_LIMIT_RPS` that is ≤0 or malformed now falls back to the default instead of silently rejecting everything.
  - Documented in docs/security.md ("Ingestion rate limiting"). `RATE_LIMIT_RPS` now means items per second.
- Verified: go build, go vet, and go test -race PASS. New tests: per-item charging (a 6-item batch, then a second batch is refused while 4 singles succeed), impossible batches, empty-batch cost, and config fallback.
- Next candidates: integration test of gateway → NATS → collector with an in-process nats-server (adds the nats-server/v2 module as a test-only dependency, which is heavy, so justify it or decide against); the empty docs (sdk-go.md, deployment.md, contributing.md); the UI has no trace-detail view of attributes or error.message.
- Blockers / needs human: `gh auth login` is still pending.

## 2026-11-03 — Span status flows end to end (was always "ok")
- Did: `SpanStatus` was on the wire but dropped at the gateway, missing from SpanInput and SpanRequest, and hard-coded to "ok" in the exporter, so failed agent steps showed as successful in the UI and in the trace_summary error_count.
  - schema: `StatusUnset`, `StatusOK`, and `StatusError` match the ClickHouse enum. `Validate` rejects anything else, including empty. An empty value would fail the enum insert and now retry forever. `New` defaults to unset.
  - gateway: `statusFromProto`. Unknown enum values from newer clients fail validation instead of being guessed.
  - telemetry: `SpanInput.Status` (empty means unset, so in-flight messages from an older gateway still work); the exporter writes the real status to ClickHouse and Tempo (OTLP code 1 or 2; unset is omitted).
  - SDK: `Span.SetOK()` and `Span.SetError(err)`. SetError(nil) is a no-op; the message goes in `error.message`, truncated on a UTF-8 rune boundary to 4096 bytes so a long error can't get the span rejected.
- Verified: go build, go vet, and go test -race PASS for root and sdk. New tests: schema status validation, the gateway enum mapping including unknown values, Process normalization, exporter row status, the Tempo OTLP status, SDK status over real mTLS, and truncateUTF8.
- Next candidates: an integration test with an in-process nats-server (gateway → NATS → collector); the empty docs (sdk-go.md, deployment.md, contributing.md); the UI has no trace-detail view of attributes or error.message; the Dockerfile hard-codes GOARCH=amd64.
- Blockers / needs human: `gh auth login` is still pending.

## 2026-11-03 — Collector pipeline: stop losing spans; no poison loops; bounded stream
- Did, fixing correctness bugs in gateway → NATS → collector → storage:
  - **Silent data loss:** `Exporter.Export` always returned nil, so a failed ClickHouse insert was acked and the span was lost. It now returns the ClickHouse error, and the message is redelivered. VictoriaMetrics and Tempo stay best-effort (logged), so a span already stored in ClickHouse isn't redelivered and duplicated because of them. Secondary writes are skipped when the primary fails.
  - **Poison loop:** undecodable messages were `Nak`'d and redelivered forever. They're now `Term`'d.
  - **Hot retry loop:** transient failures now use `NakWithDelay(5s)` instead of an immediate `Nak`.
  - **Unbounded NATS disk:** the SPANS stream used limits retention, so acked spans were kept forever. New streams are a work queue with MaxAge 7d. ⚠ JetStream can't change retention on an existing stream, so deployments with an existing SPANS stream must run `nats stream rm SPANS` to pick this up. It's documented in `spanStreamConfig`.
  - Refactored for testing: the collector logic moved to `handle()`, which returns ack, retry, or term; the exporter now depends on small backend interfaces. Removed the duplicated subject constant, and fixed the Publish comment (the payload is JSON, not proto).
- Verified: go build, go vet, and go test -race PASS for root and sdk. New tests cover every collector outcome, ClickHouse errors propagating, secondary backends being best-effort, the exporter's row mapping, `Process` (agent_name can't be spoofed, zero-time defaults, validation, LLM mapping), and the stream config. telemetry coverage went from 0% to 72%. Not tested against a real NATS or ClickHouse.
- Next candidates: **span status is dropped end to end.** proto `SpanStatus` isn't in SpanInput, and the exporter hard-codes `"ok"`. Thread it through, and add SDK `SetError`. Also: an integration test against real NATS (nats-server can run in-process), and the empty docs (sdk-go.md, deployment.md).
- Blockers / needs human: `gh auth login` is still pending. The SPANS stream recreation note above applies to existing deployments.

## 2026-11-03 — Go SDK: deliver spans, verify gateway against configured CA
- Did: `sdk/go` is a separate module that was never built (no go.sum) and wasn't covered by CI.
  - **Security bug:** `New()` ignored `TLSCACert`. With no RootCAs, it verified the gateway against the *system* roots, so a private-CA gateway failed and any publicly-trusted cert for that host was accepted. It now trusts only the configured CA and errors on a missing or unparseable CA.
  - **Functional bug:** `flush` was a no-op TODO, so spans never left the process. It now calls `IngestSpan` through the generated client with a full span to proto mapping (timestamps, attributes, LLM call). `Span.End` now returns an error on transport failure or gateway rejection; existing `s.End(ctx)` statement calls still compile.
  - go.mod moved to go 1.26.0 with toolchain go1.26.10, and tidy pulled grpc v1.83.2 and protobuf v1.36.11. Committed go.sum.
  - CI: the test and vuln jobs now build, vet, test, and govulncheck `sdk/go`.
- Verified: new end-to-end tests use a real gRPC server that requires client certs, with a CA and certs generated in the test. They cover span delivery and parent/child linking, rejection surfacing, refusing a gateway signed by a different CA, and bad CA files. sdk coverage is 84%. The root and sdk build/vet/test PASS, and govulncheck is clean for both.
- Next candidates: an SDK status API (`SetError`, since the proto `SpanStatus` is never set by the SDK or mapped by the gateway); `Span.End` being synchronous means one RPC per span, so consider batching with `IngestBatch`; tests for `telemetry.Process` and `internal/storage`; the empty docs (sdk-go.md, deployment.md).
- Blockers / needs human: `gh auth login` is still pending (the loop is using the API fallback).

## 2026-11-03 — First CI run green; loop uses gh for CI status
- CI run 41684226282 for d77f759: **all 6 jobs passed** (test, generated, vuln, and docker for gateway, collector, and ui). This confirms the Dockerfile works with committed proto code.
- Did: installed the `gh` CLI (user request). `.claude/loop.md` now checks `gh run list` (and PR checks and comments when a PR exists) during orientation, makes a CI failure priority 1, and has the loop wait on each pushed commit's run. It falls back to the public API if gh isn't authenticated.
- Blockers / needs human: run `gh auth login` once so the loop can read CI logs and PR status.
- Next candidates: wire the Go SDK client TODO (`sdk/go/sdk.go`); tests for `telemetry.Process` and `internal/storage` (0% coverage); Dockerfile GOARCH=amd64 is hard-coded.

## 2026-11-03 — CI workflow; gofmt the remaining files
- Did: added `.github/workflows/ci.yml`, which runs on every push and PR with `contents: read` and `persist-credentials: false`. Actions are pinned by commit SHA (checkout v7.0.1, setup-go v7.0.0). Jobs:
  - test: gofmt check, build, vet, test -race -cover.
  - generated: `make proto`, then fail if proto/ shows a diff or untracked files.
  - vuln: govulncheck v1.9.0.
  - docker: matrix build of gateway, collector, and ui (no push). This covers the docker build I couldn't run locally.
  - Also ran gofmt on storage/clickhouse.go, telemetry/processor.go, and sdk/go/llm.go (whitespace only), and pinned govulncheck in `make lint`.
- Verified: simulated every job locally except docker, and all pass. govulncheck reports 0 reachable vulns. The first real CI run happens on this push.
- Next candidates: check the first CI run (especially docker); wire the Go SDK client TODO (`sdk/go/sdk.go`); tests for `telemetry.Process` and `internal/storage` (both 0% coverage); Dockerfile GOARCH=amd64 is hard-coded.
- Blockers / needs human: none.

## 2026-11-03 — Commit generated proto code; loop now pushes loop/auto (requested by user)
- Decision (user): commit the generated proto code instead of gitignoring it, and have the loop push to its branch.
- Did: removed `**/*.pb.go` from .gitignore (Python output stays ignored) and committed `proto/*.pb.go`. Dropped the Dockerfile's gen-proto step and the `.dockerignore` entry, so image builds no longer need network codegen. Updated `.claude/loop.md`: after each passing commit, run `git push -u origin loop/auto`. On rejection, rebase rather than force-push. Only `loop/auto` is ever pushed.
- Verified: regeneration is deterministic (re-running `make proto` leaves no diff). go build, go vet, and go test -race ./... PASS.
- Next candidates: CI workflow (in progress before this request): build, vet, test -race, govulncheck, a **generated-code drift check** (`make proto && git diff --exit-code proto/`), and a docker build, with actions pinned by commit SHA. Then gofmt the remaining files and wire the Go SDK client TODO.
- Blockers / needs human: none.

## 2026-11-03 — Docker: generate proto code in build; add .dockerignore
- (Superseded by the next entry: generated code is now committed.) Did: the Dockerfile now runs `scripts/gen-proto.sh` after `COPY . .`, so gateway and ui images build without committed `*.pb.go`. I made the script POSIX sh with no git dependency (it finds the repo root from its own path), since the alpine builder has neither bash nor git. Added `.dockerignore`: without it, `COPY . .` copied `certs/` (private keys), `.env`, and `deployments/docker/ui-users` into the builder's layers and build cache. Also excluded .git, bin/, .claude, and stale `*.pb.go`.
- Verified: `rm proto/*.pb.go && sh scripts/gen-proto.sh` regenerates the code; go build and go test PASS. **`docker build` was not run** (loop rule: no long-running networked commands). Please check with `docker build -f deployments/docker/Dockerfile --build-arg BINARY=gateway .`
- Next candidates: add a CI workflow (go build, vet, test -race, govulncheck, plus a docker build that would cover what I couldn't verify here); gofmt storage/clickhouse.go and telemetry/processor.go; wire the Go SDK client TODO; the Dockerfile hard-codes GOARCH=amd64 (use TARGETARCH for arm64 nodes).
- Blockers / needs human: run the docker build once to confirm.

## 2026-11-03 — Fix all reachable dependency and stdlib vulnerabilities
- Did: govulncheck reachable findings went from 10 to 0. Bumped grpc v1.63.2 to v1.83.2, x/net v0.22.0 to v0.61.0, and clickhouse-go/v2 v2.23.2 to v2.50.0 (ch-go v0.61.5 to v0.74.0; the old clickhouse-go doesn't compile against the fixed ch-go), plus their transitive deps.
  - **Raised the minimum Go version from 1.22 to 1.26.** The x/net fix (v0.60.0+) requires `go 1.26.0`. Go 1.22 is long EOL, since only 1.26 and 1.27 are supported now. Added `toolchain go1.26.10` because 1.26.0–1.26.9 have reachable stdlib vulns (net/http, crypto/x509, html/template, net/url, net/textproto). GOTOOLCHAIN=auto downloads it.
  - Dockerfile builder: golang:1.22-alpine to golang:1.26.10-alpine (the old image would refuse the go directive). Updated the README tech stack to Go 1.26+.
  - Codegen pins updated to match the new runtimes: buf v1.75.0, protoc-gen-go v1.36.11, protoc-gen-go-grpc v1.6.2. Regenerated the proto code.
- Verified: go build, go vet, and go test -race ./... PASS on go1.26.10. govulncheck: "Your code is affected by 0 vulnerabilities" (3 more exist in required modules but aren't reachable). Did not run a docker build.
- Next candidates: the Dockerfile never runs gen-proto, so the gateway image build is broken (`*.pb.go` is gitignored); add the codegen step, or decide to commit the generated code. gofmt storage/clickhouse.go and telemetry/processor.go. Wire the Go SDK client TODO. Add a CI workflow (build, vet, test, and govulncheck) so vulns are caught automatically.
- Blockers / needs human: none, but note the Go 1.26 minimum for anyone building locally.

## 2026-11-03 — UI auth: login form + session cookies, and Google Cloud IAP (requested by user)
- Decision (user): use a login form with session cookies, with first-class GCP IAP support. This resolves both UI auth blockers from the previous entry.
- Did: `UI_AUTH_MODE` = login (default) | iap | none (must be set explicitly). Misconfiguration exits at startup.
  - login: htpasswd-style bcrypt users file; stateless HMAC-SHA256 session cookies (`__Host-`, HttpOnly, Secure, SameSite=Lax) that work across replicas; a dummy-hash compare for unknown users; same-origin checks on POST /login and /logout; per-user and per-IP login rate limits; open-redirect-safe `next`; htmx requests get `HX-Redirect`. Added a `ui hash-password` subcommand, since the image is FROM scratch.
  - iap: verifies `x-goog-iap-jwt-assertion` using the standard library only (ES256 against Google's JWKS, cached for 1h, unknown-kid refetch at most once a minute), checking iss, aud, exp, and iat. The unsigned email header is never trusted. Logout goes to `/_gcp_iap/clear_login_cookie`.
  - Unauthenticated `/healthz` for LB health checks (GCP's bypass IAP). The nav shows the signed-in user and a sign-out button. Templates now get the user through `handlers.render`, which also logs template errors.
  - Removed the old Bearer-token middleware and TokenStore, and the `UI_API_TOKEN` setting.
  - Deployments: compose mounts `deployments/docker/ui-users` (gitignored); the k8s ui Deployment uses login mode with secret-mounted users and probes; new `deployments/k8s/gke-iap.yaml` (BackendConfig with IAP and a /healthz health check, a NEG Service, a ManagedCertificate, and an Ingress). Wrote the UI auth section of `docs/security.md`.
  - `golang.org/x/crypto` (bcrypt) went from indirect to direct. It was already in the module graph at v0.21.0, so no new modules, and govulncheck found no reachable x/crypto vulns.
- Verified: go build, go vet, and go test -race ./... PASS. New tests cover the login flow, cookie flags, tampered and forged sessions, cross-origin rejection, rate limiting, safeNext, IAP valid and invalid tokens (alg confusion, wrong key, aud, iss, exp, tampering), key caching and rotation, users-file parsing ($2a/$2y, rejecting plaintext and md5), and hash-password. I smoke-tested startup misconfiguration with the real binary. **Not tested against real IAP or a browser.**
- Next candidates: **(security) govulncheck shows 10 reachable vulns in existing deps:** x/net v0.22.0 (fixed in 0.60.0), grpc v1.63.2 (fixed in 1.83.1), and ch-go v0.61.5 (fixed in 0.65.0). Bump them, which may require raising the go directive. The Dockerfile never runs gen-proto, so the gateway image build is broken. gofmt storage/clickhouse.go and telemetry/processor.go. Wire the Go SDK client TODO.
- Blockers / needs human: none.

## 2026-11-03 — UI: security headers, server timeouts, CSP-safe templates
- Reviewed: the UI and query API. ClickHouse queries are parameterized and pages render through html/template, so no SQLi or XSS found.
- Did: added a `securityHeaders` middleware (strict CSP with no inline script or style and frame-ancestors 'none'; nosniff; X-Frame-Options DENY; no-referrer). It wraps auth, so 401 responses get the headers too. Added Read, ReadHeader, Write, and Idle timeouts to the UI's http.Server (there were none, so slow-loris was possible). Removed a redundant inline `onclick` from traces.html (traces.js already delegates clicks) and URL-encoded the trace ID there. Turned off htmx's injected indicator `<style>`, which the CSP would block. Tests: headers are present even on auth failure, and the templates contain no inline handlers, scripts, or styles.
- Verified: go build, go vet, and go test -race ./... PASS. Not verified in a real browser, because there's no ClickHouse running locally.
- Next candidates: wire the Go SDK client TODO; add tests for telemetry.Process; handle the ignored `ExecuteTemplate` errors in cmd/ui; gofmt the remaining files.
- Blockers / needs human (RESOLVED in the next entry): (1) **UI auth fails open.** With `UI_API_TOKEN` unset (the docker-compose default), the UI serves all trace data with no auth. (2) **Bearer auth doesn't work for browsers.** Browsers can't send an Authorization header on page navigation, so setting the token makes the UI unusable. This needs a design choice, such as a login form with an HttpOnly session cookie, or relying on a reverse proxy or mTLS for the UI.

## 2026-11-03 — Harden span validation at the ingestion boundary
- Did: `schema.Validate` now bounds trace_id, span_id, and parent_id (at most 64 characters of `[A-Za-z0-9_-]`, which admits hex IDs and UUIDs), rejects empty attribute keys, caps model names at 256 bytes, and requires cost_usd to be finite and non-negative. Added a table-driven test that covers every rule (internal/schema was untested before). I also ran gofmt on validator.go.
- Verified: go build, go vet, and go test -race ./... PASS.
- Next candidates: wire the Go SDK client TODO (`sdk/go/sdk.go:~112`); add tests for `telemetry.Process` (agent_name override, zero-time defaults); gofmt the remaining files (storage/clickhouse.go, telemetry/processor.go, sdk/go/llm.go); review the UI and query API for injection and auth gaps.
- Blockers / needs human: none.

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
