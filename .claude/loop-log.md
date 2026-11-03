# Conduit loop log

Newest entries first. Each iteration is driven by `.claude/loop.md`.

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
