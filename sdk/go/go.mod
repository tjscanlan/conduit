module github.com/your-org/conduit-sdk-go

go 1.22

require (
	google.golang.org/grpc v1.63.2
	google.golang.org/protobuf v1.34.1
	// Main module provides the generated proto types.
	// Run scripts/gen-proto.sh in the root before building.
	github.com/your-org/agent-observability v0.0.0
)

replace github.com/your-org/agent-observability => ../..
