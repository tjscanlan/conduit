---
title: Contributing
---

# Contributing

Thanks for helping improve Conduit. This guide covers local setup, the
checks your change has to pass, and how changes get reviewed and merged.

## Prerequisites

| Tool | Version | Used for |
|------|---------|----------|
| Go | 1.26+ (the `toolchain` line in `go.mod` pins the exact patch release) | Everything |
| Docker with Compose v2 | recent | The local stack and the smoke test |
| Python | 3.10+ | The Python SDK |
| [uv](https://docs.astral.sh/uv/) (optional) | recent | Python environments and codegen |
| OpenSSL | any | `scripts/gen-certs.sh` |

## Repository layout

```
cmd/{gateway,collector,ui}   service entry points
internal/{auth,health,schema,storage,telemetry}
proto/                        .proto sources and the generated Go code (committed)
sdk/go                        Go SDK, a separate module
sdk/python                    Python SDK (src/conduit_sdk, with generated stubs committed)
ui/                           templates and static assets, embedded into cmd/ui
deployments/{docker,k8s}      compose stack and Kubernetes manifests
scripts/                      certs, codegen, smoke test
docs/                         this documentation
```

## Checks your change must pass

CI runs all of these on every push and pull request. Run the relevant ones
locally first.

```bash
gofmt -l .                                  # must print nothing
go build ./... && go vet ./...
go test -race ./...                         # root module
(cd sdk/go && go test -race ./...)          # Go SDK (a separate module)
(cd sdk/python && python -m pytest)         # Python SDK
make lint                                   # go vet + govulncheck
```

**End-to-end smoke test.** Start the stack (see [Deployment](deployment.md)), then run:

```bash
CLICKHOUSE_PASSWORD=... UI_USER=admin UI_PASSWORD=... ./scripts/smoke-test.sh
```

It sends traces with both SDKs and checks ClickHouse, VictoriaMetrics, Tempo,
the UI and every service's health endpoint. CI runs it against a fresh stack.

## Generated code

The Go and Python protobuf stubs are committed. After editing anything in
`proto/`, run:

```bash
make proto
```

and commit the regenerated files in the same commit. `make proto` also
regenerates `docs/api.md`, the API reference, which is built from the
`.proto` files and the validation constants in `internal/schema`. Run
`make api-docs` on its own after changing only a limit. Both generators pin
their tool versions, and CI fails if the committed code differs from what
they produce.

## Schema changes

Add a new file to `internal/storage/migrations/` named
`NNNN_description.sql`, using the next number.
- **One statement per file,** and it must be idempotent: `CREATE … IF NOT
  EXISTS`, `ALTER TABLE … ADD COLUMN IF NOT EXISTS`, and so on.
  `TestMigrationsAreIdempotent` enforces this.
- **Never edit or renumber a migration that has shipped.** Applied
  migrations are checksummed, and the collector refuses to start if one
  changes.

## Conventions

- **Go:** use the standard library where you can. A new module needs a
  short justification in the PR, because the project's pitch includes "no
  supply-chain surprises". Match the style of the surrounding code.
- **Tests:** a behavior change comes with a test. Security-relevant code,
  such as auth, validation and rate limiting, needs tests for the
  rejection paths as well as the happy path.
- **Storage:** ClickHouse queries use bound parameters, never string
  interpolation of user input. Pass times as integers. Binding a
  `time.Time` positionally truncates it to whole seconds.
- **UI:**
  - **No inline scripts or styles:** the CSP forbids them, and
    `TestTemplatesHaveNoInlineScript` enforces it.
  - **Links:** build links with query strings as `template.URL` in Go.
    `html/template` re-encodes query strings interpolated into an `href`.
  - **Colors:** follow the contrast rules in [brand.md](brand.md).
- **Docs:** if you change behavior, configuration or a guarantee, update
  the matching document in `docs/` in the same PR. If you add a doc, also
  add it to `docs/_data/docs.json`, with a one-line description. That file
  drives the site nav and the agent-readable `llms.txt`, and CI fails if a
  doc is missing from it.
- **Commits:** use the form `area: what and why`, for example
  `collector: retry failed ClickHouse writes`, and keep each commit to one
  change.

## Pull requests

1. Branch from `main`.
2. Keep each PR to one coherent theme. The description should explain
   what changed and why, and call out:
   - breaking changes and upgrade steps
   - security-relevant changes
   - what you verified, and what you didn't
3. CI must be green, with all jobs passing: `test`, `generated`,
   `python-sdk`, `vuln`, `docker` and `smoke`.
4. A maintainer merges with a merge commit.

### Automated contributions

Some changes come from an automated work loop driven by `.claude/loop.md`.
It works on `loop/<feature-name>` branches and opens a PR at each stopping
point. It never merges and never pushes to `main`. Its working notes are in
`.claude/loop-log.md`. These PRs go through the same review as any other.

## Reporting security issues

Please **don't** open a public issue for a vulnerability. Report it
privately to the maintainers, through GitHub's private vulnerability
reporting for this repository. See [Security](security.md) for the threat
model.
