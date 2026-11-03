module github.com/your-org/conduit-sdk-go

go 1.26.0

toolchain go1.26.10

// The main module provides the generated proto types.
require github.com/your-org/agent-observability v0.0.0

require (
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.11
)

require (
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

replace github.com/your-org/agent-observability => ../..
