VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/atavares-klaviyo/pass-vault/internal/cli.Version=$(VERSION)

.PHONY: build test cover lint check

build:
	go build -ldflags "$(LDFLAGS)" -o bin/pvault ./cmd/pvault

test:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }
	go vet ./...
	shellcheck install.sh scripts/*.sh

check: lint test
	sh scripts/check-dep-age.sh
