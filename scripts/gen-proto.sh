#!/usr/bin/env bash
# gen-proto.sh — generates Go code from all .proto files in proto/
#
# Prerequisites:
#   brew install protobuf
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
PROTO_DIR="$ROOT/proto"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$PROTO_DIR" --go_opt=paths=source_relative \
  --go-grpc_out="$PROTO_DIR" --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR"/span.proto \
  "$PROTO_DIR"/metric.proto \
  "$PROTO_DIR"/log.proto \
  "$PROTO_DIR"/ingestion.proto

echo "Proto generation complete. Generated files in $PROTO_DIR:"
ls "$PROTO_DIR"/*.pb.go 2>/dev/null || echo "  (none — run protoc again if missing)"
