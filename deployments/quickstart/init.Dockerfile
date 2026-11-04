# conduit-init: generates the quickstart's CA, certificates, and UI login.
# Built separately from deployments/docker/Dockerfile so it downloads only
# the one module it needs (golang.org/x/crypto), keeping first builds fast.
FROM golang:1.26.10-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/conduit-init cmd/conduit-init
ARG TARGETARCH
# No BuildKit-only features: the classic builder (e.g. Colima without buildx) must work.
RUN CGO_ENABLED=0 GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/conduit-init

FROM scratch
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
