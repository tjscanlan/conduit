#!/usr/bin/env bash
# gen-proto.sh — generates Go code from all .proto files in proto/
#
# Only Go is required: buf (which bundles its own protobuf compiler) and the
# protoc-gen-go / protoc-gen-go-grpc plugins are installed at pinned versions
# into bin/tools. Plugin versions are pinned to match the runtime modules in
# go.mod (google.golang.org/protobuf, google.golang.org/grpc).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
PROTO_DIR="$ROOT/proto"
TOOLS="$ROOT/bin/tools"

BUF_VERSION=v1.75.0
PROTOC_GEN_GO_VERSION=v1.36.11
PROTOC_GEN_GO_GRPC_VERSION=v1.6.2

export GOBIN="$TOOLS"
go install "github.com/bufbuild/buf/cmd/buf@$BUF_VERSION"
go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOC_GEN_GO_VERSION"
go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$PROTOC_GEN_GO_GRPC_VERSION"

export PATH="$TOOLS:$PATH"
cd "$PROTO_DIR"
buf generate --template buf.gen.yaml

echo "Proto generation complete. Generated files in $PROTO_DIR:"
ls "$PROTO_DIR"/*.pb.go
