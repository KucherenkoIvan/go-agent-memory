# Development targets. `make help` lists them.

GOBIN := $(shell go env GOPATH)/bin

.PHONY: help run tui test lint fmt build tools

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

run: ## start the server (HTTP + gRPC)
	go run ./cmd/app

tui: ## start the terminal UI (same features, in-process)
	go run ./cmd/tui

test: ## all tests (no docker — sqlite is in-memory)
	go test ./...

lint: ## gofmt check + go vet + golangci-lint
	@fmtout=$$(gofmt -l .); if [ -n "$$fmtout" ]; then echo "gofmt needed:"; echo "$$fmtout"; exit 1; fi
	go vet ./...
	$(GOBIN)/golangci-lint run ./...

fmt: ## format everything
	gofmt -w .

build: ## static binaries into bin/
	CGO_ENABLED=0 go build -o bin/app ./cmd/app
	CGO_ENABLED=0 go build -o bin/tui ./cmd/tui

tools: ## install dev tools into GOPATH/bin
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
