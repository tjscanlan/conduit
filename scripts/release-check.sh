#!/usr/bin/env bash
# Checks that the tree is consistent for a release tag, without touching the
# network. Usage: scripts/release-check.sh v0.1.0 [--notes FILE]
# With --notes, also writes that version's CHANGELOG section to FILE.
set -euo pipefail

tag="${1:-}"
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
  echo "usage: $0 vX.Y.Z[-pre] [--notes FILE]" >&2
  exit 2
fi
version="${tag#v}"
root="$(cd "$(dirname "$0")/.." && pwd)"
fail=0

py_version="$(sed -n 's/^version = "\(.*\)"$/\1/p' "$root/sdk/python/pyproject.toml" | head -1)"
# PyPI normalizes pre-releases (0.2.0-rc.1 -> 0.2.0rc1), so accept that form too.
py_want="$(printf '%s' "$version" | sed -E 's/-//; s/\.([0-9]+)$/\1/')"
case "$version" in
  *-*) [ "$py_version" = "$py_want" ] || { echo "sdk/python version is $py_version, want $py_want"; fail=1; } ;;
  *)   [ "$py_version" = "$version" ] || { echo "sdk/python version is $py_version, want $version"; fail=1; } ;;
esac

grep -q "^require github.com/tjscanlan/conduit $tag\$" "$root/sdk/go/go.mod" \
  || { echo "sdk/go/go.mod must require github.com/tjscanlan/conduit $tag"; fail=1; }

notes="$(awk -v v="$version" '
  index($0, "## [" v "]") == 1 { on = 1; next }
  on && /^## \[/ { exit }
  on && /^\[[^]]+\]: / { exit }
  on { print }
' "$root/CHANGELOG.md" | sed -e '/./,$!d')"
[ -n "$notes" ] || { echo "CHANGELOG.md has no section for [$version]"; fail=1; }

for image in $(grep -o 'ghcr.io/tjscanlan/conduit-[a-z]*:[^ ]*' "$root/deployments/k8s/deployment.yaml"); do
  [ "${image##*:}" = "$tag" ] || { echo "deployments/k8s/deployment.yaml uses $image, want :$tag"; fail=1; }
done

[ "$fail" -eq 0 ] || exit 1

if [ "${2:-}" = "--notes" ]; then
  printf '%s\n' "$notes" > "$3"
fi
echo "release $tag: consistent"
