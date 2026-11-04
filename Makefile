.PHONY: build test lint proto roadmap api-docs certs docker clean

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/gateway ./cmd/gateway
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/collector ./cmd/collector
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ui ./cmd/ui

test:
	go test -race -cover ./...

lint:
	go vet ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

proto:
	./scripts/gen-proto.sh
	./scripts/gen-proto-python.sh
	python3 scripts/gen-api-docs.py

roadmap:
	python3 scripts/gen-roadmap.py

api-docs:
	python3 scripts/gen-api-docs.py

certs:
	./scripts/gen-certs.sh

docker:
	docker compose -f deployments/docker/docker-compose.yml up --build

clean:
	rm -rf bin/
	rm -f certs/*.key certs/*.crt certs/*.csr certs/*.srl
