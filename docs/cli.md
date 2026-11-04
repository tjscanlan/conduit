# conduitctl

`conduitctl` runs a local Conduit with one command and helps connect agents
to it. It needs only Docker: the quickstart's compose files are built into
the binary, and the released, signed images are pulled.

## Install

Download the archive for your platform from the
[latest release](https://github.com/tjscanlan/conduit/releases/latest), check
it, and put `conduitctl` on your `PATH`:

```bash
# Linux and macOS; pick linux|darwin and amd64|arm64
V=v0.2.0 OS=darwin ARCH=arm64
curl -LO https://github.com/tjscanlan/conduit/releases/download/$V/conduitctl_${V}_${OS}_${ARCH}.tar.gz
curl -LO https://github.com/tjscanlan/conduit/releases/download/$V/SHA256SUMS
curl -LO https://github.com/tjscanlan/conduit/releases/download/$V/SHA256SUMS.sigstore.json
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "https://github.com/tjscanlan/conduit/.github/workflows/release.yml@refs/tags/$V" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com   # optional, recommended
shasum -a 256 -c SHA256SUMS --ignore-missing
tar -xzf conduitctl_${V}_${OS}_${ARCH}.tar.gz && sudo mv conduitctl_${V}_${OS}_${ARCH}/conduitctl /usr/local/bin/
```

Or, with Go: `go install github.com/tjscanlan/conduit/cmd/conduitctl@latest`.

`conduitctl` ships with releases after v0.1.0: v0.1.0 has no binaries, and
its images lack the two quickstart helpers (`conduit-init`,
`conduit-demo-agent`). Until then, use `make quickstart` from a checkout;
it runs the same stack.

## Commands

| Command | What it does |
|---|---|
| `conduitctl quickstart` | Writes the compose bundle and generated secrets to `~/.conduit/quickstart`, starts the stack, waits for health and the first demo trace, and prints the UI login and the `CONDUIT_*` settings for your agent. Running it again restarts with the same data and credentials. |
| `conduitctl status` | Shows the services, the login, and the settings again. |
| `conduitctl agent-cert NAME` | Issues a client certificate for agent `NAME`, signed by the quickstart's CA, and prints how to use it. Valid existing certificates are kept. |
| `conduitctl doctor` | Checks Docker (running, Compose v2, at least 2 GiB of memory), free ports, every service's health, the UI, the gateway (an mTLS call as `my-agent`), and that demo data reached ClickHouse. Each failure says what to do. |
| `conduitctl down` | Stops the stack, keeping data and credentials. |
| `conduitctl reset` | Stops the stack and deletes its data and credentials. |
| `conduitctl version` | Prints the version; images default to the same tag. |

Flags (all commands): `-dir` (default `~/.conduit/quickstart`), `-ui-port`
(default 8080), `-gateway-port` (default 4317), `-images` (image tag; default
is the CLI's own version), and `-no-demo` (skip the simulated `demo-agent`).

## Things to know

- **Local evaluation only:** the CA key sits next to the certificates it
  signs, and only `127.0.0.1` ports are opened. For anything shared, see
  [Deployment](deployment.md).
- **One local stack at a time:** `conduitctl` and `make quickstart` run the
  same compose project (`conduit-quickstart`) and share its data volumes. So
  `docker compose -p conduit-quickstart logs gateway` works with either.
- **Keep `-dir` under your home directory:** Docker Desktop and Colima share
  only some host folders with containers, usually your home.
- **Development builds:** a `conduitctl` built from a checkout reports
  version `dev` and needs `-images`. `scripts/build-quickstart-images.sh
  ci-local` builds every image locally for `-images ci-local`; CI tests the
  CLI this way.
