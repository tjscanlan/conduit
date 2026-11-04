#!/bin/sh
# gen-proto-python.sh — generates the Python SDK's protobuf/gRPC stubs into
# sdk/python/src/conduit_observability/_proto from proto/*.proto.
#
# Uses grpcio-tools (which bundles protoc) at a pinned version, via uv when
# available, otherwise from the current python3 environment. The pin must be
# compatible with the SDK's protobuf/grpcio runtime ranges in pyproject.toml.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/sdk/python/src/conduit_observability/_proto"
GRPCIO_TOOLS_VERSION=1.85.1

if command -v uv >/dev/null 2>&1; then
  PROTOC="uv run --quiet --no-project --python 3.12 --with grpcio-tools==$GRPCIO_TOOLS_VERSION python -m grpc_tools.protoc"
else
  PROTOC="python3 -m grpc_tools.protoc"
fi

rm -f "$OUT"/*_pb2.py "$OUT"/*_pb2_grpc.py "$OUT"/*_pb2.pyi
$PROTOC -I "$ROOT/proto" \
  --python_out="$OUT" --pyi_out="$OUT" --grpc_python_out="$OUT" \
  "$ROOT"/proto/span.proto "$ROOT"/proto/metric.proto "$ROOT"/proto/log.proto "$ROOT"/proto/ingestion.proto

# protoc emits top-level imports ("import span_pb2 as ..."); make them
# package-relative so the stubs work inside conduit_observability._proto.
for f in "$OUT"/*_pb2.py "$OUT"/*_pb2_grpc.py "$OUT"/*_pb2.pyi; do
  sed -i.bak -E 's/^import ([a-z_]+_pb2) as /from . import \1 as /; s/^from ([a-z_]+_pb2) import /from .\1 import /' "$f"
  rm -f "$f.bak"
done

echo "Python stubs generated in $OUT:"
ls "$OUT"
