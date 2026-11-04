# demo-agent: the quickstart's simulated sample agent (sdk/go/examples/demo-agent).
FROM golang:1.26.10-alpine AS build
WORKDIR /src
# The SDK module replaces the main module with ../.. for the generated protos.
COPY go.mod go.sum ./
COPY proto proto
COPY sdk/go sdk/go
WORKDIR /src/sdk/go
ARG TARGETARCH
# No BuildKit-only features: the classic builder (e.g. Colima without buildx) must work.
RUN CGO_ENABLED=0 GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -trimpath -ldflags="-s -w" -o /out/app ./examples/demo-agent

FROM scratch
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
