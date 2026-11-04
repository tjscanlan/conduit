#!/usr/bin/env bash
# build-quickstart-images.sh TAG — build every image the quickstart runs from
# this checkout, named as the release names them, for testing conduitctl
# before a release (e.g. TAG=ci-local, then `conduitctl quickstart -images ci-local`).
# Builds one at a time so it fits a 2 GiB Docker VM. Never pushes.
set -euo pipefail
tag="${1:?usage: $0 TAG}"
case "$tag" in v[0-9]*) echo "refusing to shadow a release tag ($tag) locally; use something like ci-local" >&2; exit 2 ;; esac
cd "$(dirname "$0")/.."
r=ghcr.io/tjscanlan
for b in gateway collector ui; do
  docker build -q -f deployments/docker/Dockerfile --build-arg BINARY=$b -t "$r/conduit-$b:$tag" .
done
docker build -q -f deployments/quickstart/init.Dockerfile -t "$r/conduit-init:$tag" .
docker build -q -f deployments/quickstart/demo-agent.Dockerfile -t "$r/conduit-demo-agent:$tag" .
echo "built $r/conduit-{gateway,collector,ui,init,demo-agent}:$tag"
