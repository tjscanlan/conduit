# Deployment

Conduit runs as three binaries, `gateway`, `collector` and `ui`. Each is
shipped as a `FROM scratch` image built from `deployments/docker/Dockerfile`
with `--build-arg BINARY=<name>`. The binaries depend on NATS JetStream,
ClickHouse, VictoriaMetrics and Tempo.

| Component | Port | Talks to |
|-----------|------|----------|
| gateway | 4317 (gRPC, mTLS); 8081 health | NATS |
| collector | 8081 health | NATS, ClickHouse, VictoriaMetrics, Tempo (OTLP/HTTP :4318) |
| ui | 8080 (HTTP; `/healthz` and `/readyz` unauthenticated) | ClickHouse |

## Health checks

Every service exposes unauthenticated health endpoints over plain HTTP:

| Endpoint | Meaning | Gateway / collector | UI |
|----------|---------|---------------------|----|
| `/livez` (UI: `/healthz`) | The process is up | `HEALTH_ADDR`, default `:8081` | main port, 8080 |
| `/readyz` | Dependencies are reachable: NATS for the gateway; NATS and ClickHouse for the collector; ClickHouse for the UI | `HEALTH_ADDR` | main port |

- **Why a separate port for the gateway:** its gRPC port requires mTLS client
  certificates, which probes can't present.
- **What `/readyz` returns:** a 200 or a 503, with each check reported only
  as `ok` or `unavailable`. The underlying errors go to the service log, so
  the endpoint never exposes hostnames or credentials.
- **Health checks in scratch images:** the images have no shell or curl, so
  each binary includes `/app healthcheck`, which calls its own `/readyz` and
  exits 0 or 1. Compose health checks use it.
- **Keep `HEALTH_ADDR` internal:** don't expose it outside the cluster.

## Local: quickstart

`make quickstart` (or `deployments/quickstart/quickstart.sh`) runs the
released images with nothing but Docker:

- **Credentials:** the one-shot `conduit-init` container writes a dev CA, the
  gateway certificate, client certificates for `demo-agent` and `my-agent`,
  and a random UI login into `deployments/quickstart/.conduit/` (gitignored).
  It runs as your user, so the agent keys are readable by your own code, and
  it keeps existing files on later runs.
- **Secrets:** the script writes a random ClickHouse password and session key
  to `deployments/quickstart/.env` (gitignored, owner-only).
- **Network:** only the UI (`127.0.0.1:8080`) and the gateway
  (`127.0.0.1:4317`) listen, on loopback. Change the ports with
  `CONDUIT_UI_PORT` and `CONDUIT_GATEWAY_PORT`.
- **Small machines:** ClickHouse uses a low-memory profile and system log
  tables are off, so the stack fits a 2 GiB Docker VM. Span retention is
  7 days.
- **Demo data:** a simulated `demo-agent` (no model calls; every span has
  `demo=true`) sends a run every 8 seconds. Start without it using
  `CONDUIT_DEMO_AGENT=0`, or stop it with
  `docker compose -f deployments/quickstart/compose.yml stop demo-agent`.
- **Version:** images default to `v0.1.0`; set `CONDUIT_VERSION` to pick
  another release.

`quickstart.sh status` reprints the login, `down` stops the stack, and
`reset` deletes its data and credentials. `scripts/quickstart-check.sh` is
the CI check: from a cold start, the first trace must be visible in the UI
within 300 seconds.

The quickstart is for local evaluation only. Its CA key sits next to the
certificates it signs; use real PKI (below) for anything shared.

## Local: docker compose (from source)

For working on Conduit itself, this stack builds the images from your
checkout.

```bash
./scripts/gen-certs.sh                         # dev CA, gateway cert, example agent cert
./scripts/gen-certs.sh agent my-agent          # more agents as needed

# UI login (see security.md for other auth modes)
printf '%s' 'a-long-dev-password' | go run ./cmd/ui hash-password \
  | sed 's/^/admin:/' > deployments/docker/ui-users

export CLICKHOUSE_PASSWORD=$(openssl rand -hex 16)
docker compose -f deployments/docker/docker-compose.yml up --build
```

Then open http://localhost:8080 and sign in as `admin`. Agents connect to
`localhost:4317` using `certs/ca.crt` and their own client cert (see
[sdk-go.md](sdk-go.md)).

To check the whole pipeline end to end, run
`UI_USER=admin UI_PASSWORD=… ./scripts/smoke-test.sh` with the same
`CLICKHOUSE_PASSWORD` exported. It sends a trace with the Go SDK and checks
that it reaches ClickHouse, VictoriaMetrics, Tempo and the UI. CI runs it on
every push.

Compose sets `UI_COOKIE_INSECURE=true` because it serves plain HTTP on
localhost. Never set it anywhere else. Without `UI_SESSION_KEY`, sessions
end whenever the UI restarts.

## Kubernetes

The manifests in `deployments/k8s/` assume a `conduit` namespace,
[cert-manager](https://cert-manager.io), and NATS, ClickHouse,
VictoriaMetrics and Tempo reachable at the hostnames in `configmap.yaml`.

### 1. Certificates (`mtls-cert.yaml`)

The file creates a root CA, `conduit-ca-issuer`, a gateway server cert
(secret `conduit-mtls-certs`) and an example agent cert. The gateway and
the agents all chain to that one CA. The gateway Deployment maps the
secret's `tls.crt`, `tls.key` and `ca.crt` onto `server.crt`, `server.key`
and `ca.crt`.

For production, swap the bootstrap self-signed root CA for your PKI but
keep the issuer name. Issue one `Certificate` per agent with
`usages: [client auth]`. Its `commonName` becomes the agent's identity.

### 2. Secrets

Create `conduit-secrets` in the `conduit` namespace:

| Key | Used by | Value |
|-----|---------|-------|
| `clickhouse-dsn` | collector, ui | `clickhouse://user:pass@host:9000/observability` |
| `ui-users` | ui (login mode) | lines of `username:<output of ui hash-password>` |
| `ui-session-key` | ui (login mode) | 32 or more random bytes, for example `openssl rand -hex 32` |

```bash
kubectl -n conduit create secret generic conduit-secrets \
  --from-literal=clickhouse-dsn="$CLICKHOUSE_DSN" \
  --from-file=ui-users=./ui-users \
  --from-literal=ui-session-key="$(openssl rand -hex 32)"
```

### 3. Apply

```bash
kubectl apply -f deployments/k8s/configmap.yaml \
              -f deployments/k8s/mtls-cert.yaml \
              -f deployments/k8s/deployment.yaml \
              -f deployments/k8s/service.yaml
```

The gateway Service is a ClusterIP. Expose it through a LoadBalancer or a
TCP/gRPC passthrough if agents run outside the cluster. TLS must terminate
at the gateway itself, or mTLS identity is lost.

On GKE, use `deployments/k8s/gke-iap.yaml` instead of the `ui` Service to
put the UI behind Identity-Aware Proxy (see [security.md](security.md#iap-mode-google-cloud)).

### Upgrading an existing deployment

- **Schema migrations:** the collector applies pending migrations at
  startup. To migrate before rolling out new collectors, for example in a
  Kubernetes Job, run `/app migrate` with the collector image and its
  environment. Never edit a shipped migration file; the collector refuses
  to start if a checksum changes.

- **NATS stream:** the SPANS stream now uses work-queue retention, so
  acked spans are deleted. JetStream can't change the retention of an
  existing stream. Once, with the gateway and collector stopped, run
  `nats stream rm SPANS` so it's recreated with the new settings.
- **UI auth:** `UI_API_TOKEN` is gone. Configure `UI_AUTH_MODE` and its
  settings (see [security.md](security.md#ui-authentication)).
- **Tempo endpoint:** `TEMPO_ENDPOINT` must point at Tempo's **OTLP/HTTP**
  port, `:4318`. The collector posts to `/v1/traces`. Port `:4317` is
  OTLP/gRPC and won't work.

## Configuration reference

| Variable | Component | Default |
|----------|-----------|---------|
| `GATEWAY_ADDR` | gateway | `:4317` |
| `TLS_CERT`, `TLS_KEY`, `TLS_CA` | gateway | `/etc/certs/server.crt`, `server.key`, `ca.crt` |
| `RATE_LIMIT_RPS` | gateway | `100` telemetry items per second per agent |
| `HEALTH_ADDR` | gateway, collector | `:8081` (liveness and readiness endpoints) |
| `SPAN_RETENTION_DAYS` | collector | `90` (1–3650). The collector reconciles the spans table's TTL on startup; shortening it deletes data that's now past retention. |
| `STORE_LLM_PAYLOADS` | gateway | `true`. Set `false` to drop prompts and completions at ingestion; model, tokens and cost are kept. An unrecognized value stops startup. |
| `NATS_URL` | gateway, collector | `nats://localhost:4222` |
| `CLICKHOUSE_DSN` | collector, ui | local dev DSN. Always set it explicitly. Add `?secure=true` for TLS to ClickHouse. |
| `VICTORIAMETRICS_URL` | collector | `http://localhost:8428` |
| `TEMPO_ENDPOINT` | collector | `http://localhost:4318` |
| `LISTEN_ADDR` | ui | `:8080` |
| `UI_AUTH_MODE` and related | ui | see [security.md](security.md#ui-authentication) |
