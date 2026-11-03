# Deployment

Conduit runs as three binaries, `gateway`, `collector` and `ui`. Each is
shipped as a `FROM scratch` image built from `deployments/docker/Dockerfile`
with `--build-arg BINARY=<name>`. The binaries depend on NATS JetStream,
ClickHouse, VictoriaMetrics and Tempo.

| Component | Port | Talks to |
|-----------|------|----------|
| gateway | 4317 (gRPC, mTLS) | NATS |
| collector | none | NATS, ClickHouse, VictoriaMetrics, Tempo (OTLP/HTTP :4318) |
| ui | 8080 (HTTP; `/healthz` unauthenticated) | ClickHouse |

## Local: docker compose

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
| `NATS_URL` | gateway, collector | `nats://localhost:4222` |
| `CLICKHOUSE_DSN` | collector, ui | local dev DSN. Always set it explicitly. |
| `VICTORIAMETRICS_URL` | collector | `http://localhost:8428` |
| `TEMPO_ENDPOINT` | collector | `http://localhost:4318` |
| `LISTEN_ADDR` | ui | `:8080` |
| `UI_AUTH_MODE` and related | ui | see [security.md](security.md#ui-authentication) |
