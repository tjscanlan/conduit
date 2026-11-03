# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

## 2026-11-03 — Fix smoke CI: stale tracked certs + gen-certs reissue bug
- CI run 41698657967 failed in `compose smoke test` at gen-certs: "certs/agent-example.crt already exists".
  - Cause 1: `certs/{ca,server,agent-example}.crt` had been tracked since the initial "mvp-ish code" commit, before `certs/` was gitignored. They're **public certificates only**. I scanned all history: no private key was ever committed (the only "PRIVATE KEY" matches are PEM labels in sdk_test.go). They're useless without their keys, so I untracked them; local copies stay.
  - Cause 2 (my bug from 388d75b): a full `gen-certs.sh` run creates a new CA, then refused to reissue agent-example because a cert already existed. That cert was signed by the old CA, so it was invalid. The full run now always reissues the example agent cert.
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
