# Development targets. `make help` lists them.

GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)
INSTALL_BIN ?= $(GOBIN)/recall
RECALL_DB   ?= $(HOME)/.local/share/recall/memory.db
KEEP_BACKUPS := 5

.PHONY: help build test lint fmt tools install

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## static binary into bin/ (version-stamped from git describe)
	CGO_ENABLED=0 go build -ldflags "-X main.version=$$(git describe --tags --always)" -o bin/recall ./cmd/recall

install: build ## build, warn about live processes, backup the db, replace $(INSTALL_BIN)
	@if pgrep -f 'recall run' > /dev/null; then \
		echo "⚠ running recall processes keep the OLD binary until their session restarts:"; \
		pgrep -fl 'recall run' | sed 's/^/    /'; \
	fi
	@if [ -f "$(RECALL_DB)" ] && ! command -v sqlite3 > /dev/null; then \
		echo "⚠ sqlite3 not found: skipping the db backup"; \
	elif [ -f "$(RECALL_DB)" ]; then \
		backup="$(RECALL_DB).bak.$$(date +%Y%m%d-%H%M%S)"; \
		sqlite3 "$(RECALL_DB)" "VACUUM INTO '$$backup'" && echo "db backed up: $$backup"; \
		ls -t "$(RECALL_DB)".bak.* 2>/dev/null | tail -n +$$(( $(KEEP_BACKUPS) + 1 )) | xargs rm -f; \
	fi
	@# rm first, never overwrite in place: macOS SIGKILLs an executable whose
	@# inode contents changed (stale code-signature cache)
	@mkdir -p "$(dir $(INSTALL_BIN))"
	@rm -f "$(INSTALL_BIN)" && cp bin/recall "$(INSTALL_BIN)"
	@"$(INSTALL_BIN)" --version

test: ## all tests (no docker — sqlite is in-memory)
	go test ./...

lint: ## gofmt check + go vet + golangci-lint
	@fmtout=$$(gofmt -l .); if [ -n "$$fmtout" ]; then echo "gofmt needed:"; echo "$$fmtout"; exit 1; fi
	go vet ./...
	$(GOBIN)/golangci-lint run ./...

fmt: ## format everything
	gofmt -w .

tools: ## install dev tools into GOPATH/bin
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
