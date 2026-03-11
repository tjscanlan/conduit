# Multi-stage build — final image is FROM scratch (no OS, no shell, just the binary)

# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build all three binaries — static, no CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /out/gateway ./cmd/gateway

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /out/collector ./cmd/collector

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /out/ui ./cmd/ui

# Stage 2: Minimal runtime image — zero attack surface
FROM scratch

# Copy CA certs for outbound TLS (connecting to ClickHouse, NATS, etc.)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary (choose which one to run via ARG or separate images)
ARG BINARY=gateway
COPY --from=builder /out/${BINARY} /app

ENTRYPOINT ["/app"]
