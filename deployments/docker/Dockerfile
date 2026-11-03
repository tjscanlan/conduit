# Multi-stage build — final image is FROM scratch (no OS, no shell, just the binary)

# Stage 1: Build
FROM golang:1.26.10-alpine AS builder

WORKDIR /build

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build only the requested binary, static with no CGO. GOARCH follows the
# target platform when BuildKit provides it (docker buildx --platform ...),
# otherwise the builder's native architecture, so arm64 hosts get arm64 images.
ARG BINARY=gateway
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build \
    -trimpath -ldflags="-s -w" \
    -o /out/app ./cmd/${BINARY}

# Stage 2: Minimal runtime image — zero attack surface
FROM scratch

# Copy CA certs for outbound TLS (connecting to ClickHouse, NATS, etc.)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /out/app /app

ENTRYPOINT ["/app"]
