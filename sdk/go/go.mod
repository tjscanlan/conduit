module github.com/tjscanlan/conduit/sdk/go

go 1.26.0

toolchain go1.26.10

// The main module provides the generated proto types. The replace below is
// for development in this repo; consumers resolve the tagged version instead.
require github.com/tjscanlan/conduit v0.2.0

require (
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.11
)

require (
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

replace github.com/tjscanlan/conduit => ../..
