.PHONY: build test lint proto certs docker clean

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/gateway ./cmd/gateway
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/collector ./cmd/collector
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ui ./cmd/ui

test:
	go test -race -cover ./...

lint:
	go vet ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

proto:
	./scripts/gen-proto.sh

certs:
	./scripts/gen-certs.sh

docker:
	docker compose -f deployments/docker/docker-compose.yml up --build

clean:
	rm -rf bin/
	rm -f certs/*.key certs/*.crt certs/*.csr certs/*.srl
