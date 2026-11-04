# Releasing

How Conduit is versioned, how to verify what you download, how to upgrade, and
how a maintainer cuts a release.

## Versioning policy

Conduit uses [Semantic Versioning](https://semver.org/). It is **pre-1.0**: a
minor version (`0.x.0`) may contain breaking changes, and a patch version
(`0.x.y`) will not. Breaking changes are called out in the
[changelog](https://github.com/tjscanlan/conduit/blob/main/CHANGELOG.md).

The policy covers this public API:

- The gRPC ingestion API in `proto/` ([reference](api.md))
- The exported APIs of the Go and Python SDKs
- Environment variables and subcommands ([configuration reference](deployment.md#configuration-reference))

It does not cover packages under `internal/`, the root Go module's other packages
(that module runs the services and provides generated types to the SDK), the UI's
HTML, or the exact ClickHouse schema. Schema changes reach existing data through
migrations, which the collector applies at startup.

All artifacts of a release share one version number.

| Artifact | Where | Version form |
|----------|-------|--------------|
| Images | `ghcr.io/tjscanlan/conduit-{gateway,collector,ui}` | `v0.1.0` |
| Go SDK | `github.com/tjscanlan/conduit/sdk/go` | tag `sdk/go/v0.1.0`, used as `@v0.1.0` |
| Python SDK | [`conduit-observability`](https://pypi.org/project/conduit-observability/) | `0.1.0` |

## Verifying images

Images are built by the `release` workflow and signed with keyless
[cosign](https://docs.sigstore.dev/), so a valid signature shows the image came
from this repository's workflow at a version tag:

```bash
cosign verify ghcr.io/tjscanlan/conduit-gateway:v0.1.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/tjscanlan/conduit/\.github/workflows/release\.yml@refs/tags/v'
```

Each image also carries an SBOM and build provenance as attestations in its
manifest list. To see them:

```bash
docker buildx imagetools inspect ghcr.io/tjscanlan/conduit-gateway:v0.1.0 --raw
```

In production, deploy by digest rather than by tag.

## Upgrading

1. Read the changelog entries between your version and the new one.
2. Follow [Upgrading an existing deployment](deployment.md#upgrading-an-existing-deployment).

## Cutting a release

Only a maintainer pushes tags.

### One-time setup

On PyPI, add a trusted publisher for the project `conduit-observability`: owner
`tjscanlan`, repository `conduit`, workflow `release.yml`, environment `pypi`. If
the project doesn't exist yet, add it as a *pending* publisher in your account
settings. If it does, add it in the project's settings. Consider also requiring a
reviewer on the `pypi` environment in the repository settings.

### Each release

1. On a branch, set the new version in all of these, then merge:
   - `sdk/python/pyproject.toml`
   - the `require github.com/tjscanlan/conduit` line in `sdk/go/go.mod`
   - the image tags in `deployments/k8s/deployment.yaml`
   - `CHANGELOG.md`: move `[Unreleased]` into an `[X.Y.Z]` section and update the
     links at the bottom

   `scripts/release-check.sh vX.Y.Z` checks that these agree, and CI runs it.
2. Tag the merge commit and push both tags at once. The SDK requires the root
   module at the same version, and the workflow refuses a mismatch:

   ```bash
   git switch main && git pull
   git tag v0.1.0 && git tag sdk/go/v0.1.0
   git push --atomic origin v0.1.0 sdk/go/v0.1.0
   ```

3. Watch the `release` workflow. It tests the commit, builds, pushes and signs the
   images, publishes to PyPI, checks that the Go SDK resolves the way a consumer
   would fetch it, and creates the GitHub Release from the changelog section.

A failed run can be re-run: images are rebuilt and re-signed, and PyPI files that
already exist are skipped. PyPI versions and Go module tags are permanent, so fix a
bad release with a new patch version rather than by moving a tag.
