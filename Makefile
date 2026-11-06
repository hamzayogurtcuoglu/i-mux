# imux build tooling. See TASKS.md (T0.7).

BINARY    ?= imux
PKG       := github.com/hamzayogurtcuoglu/i-mux
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)
FUZZTIME  ?= 60s
GO        ?= go

export CGO_ENABLED = 0

.PHONY: all build install test race e2e lint fmt bench fuzz-short check-fixtures size clean generate

all: lint test build

build: ## Build bin/imux
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/imux

install: ## Install imux into GOBIN
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/imux

test: ## Unit tests
	$(GO) test ./...

race: ## Unit tests with the race detector (needs cgo)
	CGO_ENABLED=1 $(GO) test -race ./...

e2e: build ## End-to-end tests against a real tmux on a private socket
	IMUX_E2E_BIN=$(CURDIR)/bin/$(BINARY) $(GO) test -tags e2e -count=1 -timeout 15m ./e2e/...

lint: ## golangci-lint (vet, staticcheck, errcheck, gofumpt, misspell)
	golangci-lint run ./...

fmt: ## Format with gofumpt via golangci-lint
	golangci-lint fmt ./...

bench: ## Benchmarks
	$(GO) test -run '^$$' -bench . -benchmem ./...

fuzz-short: ## Run every fuzz target for FUZZTIME (default 60s)
	FUZZTIME=$(FUZZTIME) ./scripts/fuzz-short.sh

generate: ## Regenerate generated files (docs/CONFIG.md, ...)
	$(GO) generate ./...

check-fixtures: ## Fail on real paths, secrets or emails in testdata/
	./scripts/check-fixtures.sh

size: build ## Fail if the stripped binary exceeds 15 MB
	@size=$$(wc -c < bin/$(BINARY)); \
	echo "bin/$(BINARY): $$size bytes"; \
	test $$size -le 15728640 || { echo "binary larger than 15 MB"; exit 1; }

clean:
	$(GO) clean
	rm -f bin/$(BINARY)
