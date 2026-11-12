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
PKGS      ?= ./...

export CGO_ENABLED = 0

.PHONY: all build build-windows install test race e2e lint fmt bench fuzz-short check-fixtures size clean generate \
	ci lint-all generate-check shellcheck test-macos test-windows

all: lint test build

build: ## Build bin/imux
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/imux

# On Windows without make: scripts/build-windows.ps1 does the same.
build-windows: ## Build bin/imux.exe for Windows (GOARCH=arm64 for ARM)
	GOOS=windows $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY).exe ./cmd/imux

# A new file, then mv: overwriting a running imux in place makes macOS kill
# every imux process (Code Signature Invalid), the sidebars included.
INSTALL_DIR ?= $(HOME)/.local/bin
install-local: build ## Install bin/imux into ~/.local/bin the safe way (a new file, then mv)
	mkdir -p "$(INSTALL_DIR)"
	cp bin/$(BINARY) "$(INSTALL_DIR)/.$(BINARY).new.$$$$" && mv -f "$(INSTALL_DIR)/.$(BINARY).new.$$$$" "$(INSTALL_DIR)/$(BINARY)"
	"$(INSTALL_DIR)/$(BINARY)" version

install: ## Install imux into GOBIN
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/imux

test: ## Unit tests
	$(GO) test $(PKGS)

race: ## Unit tests with the race detector (needs cgo)
	CGO_ENABLED=1 $(GO) test -race $(PKGS)

# CI runs the unit tests on Linux, macOS and Windows; a test that passes here
# can fail there. `make ci` runs what CI runs, as close as this machine can:
# run it before a push (.agents/skills/check-before-push).
ci: lint-all generate-check check-fixtures shellcheck race test-macos test-windows size e2e ## Every check CI runs, here: run it before a push

lint-all: ## Lint and vet as CI does: Linux (e2e tests too), Windows and macOS builds
	golangci-lint run ./...
	GOOS=windows golangci-lint run ./cmd/... ./internal/...
	$(GO) vet ./... && $(GO) vet -tags e2e ./...
	GOOS=windows $(GO) vet ./cmd/... ./internal/...
	GOOS=darwin $(GO) vet ./...

generate-check: ## Fail if go generate changes a file (CI: "Generated files are current")
	@before=$$( (git diff --no-ext-diff; git status --porcelain=v1 -uall) | cksum ); \
	$(GO) generate ./... || exit 1; \
	after=$$( (git diff --no-ext-diff; git status --porcelain=v1 -uall) | cksum ); \
	if [ "$$before" != "$$after" ]; then \
		echo "generate-check: go generate changed files (regenerated now): review and commit them"; git status --short; exit 1; \
	fi

shellcheck: ## shellcheck the shell scripts as CI does (skipped, and said, when shellcheck is missing)
	@if command -v shellcheck >/dev/null; then \
		shellcheck -s sh install.sh scripts/*.sh scripts/screenshots/*.sh e2e/fakeagents/fake.sh e2e/fakeagents/fake-* e2e/fakeagents/nvim; \
	else \
		echo "shellcheck: not installed, scripts NOT checked (CI checks them; pip install shellcheck-py)"; \
	fi

# macOS's temp folder is behind a symlink (/var is /private/var) and git,
# os.Getwd and filepath.EvalSymlinks give the resolved path: a test that
# compares the two passes on Linux and fails on macOS. Here TMPDIR goes
# through a symlink too.
test-macos: ## Unit tests with TMPDIR behind a symlink, as on macOS
	@real=$$(mktemp -d) && ln -s "$$real" "$$real.link" && \
	trap 'rm -rf "$$real" "$$real.link"' EXIT && \
	TMPDIR="$$real.link" $(GO) test -count=1 $(PKGS)

# In WSL the Windows test binaries run on the Windows host through interop,
# as on CI's Windows runner (PowerShell commands, 8.3 temp paths, CRLF), with
# Git for Windows' sh and git and a throwaway user profile
# (scripts/wsl-exec.sh). Elsewhere only the Windows build is vetted.
test-windows: ## Unit tests on Windows: from WSL on the Windows host; elsewhere vets the Windows build only
	@if grep -qs '^enabled' /proc/sys/fs/binfmt_misc/WSLInterop /proc/sys/fs/binfmt_misc/WSLInterop-late; then \
		wintemp=$$(cd /mnt/c && cmd.exe /d /c 'echo %TEMP%' | tr -d '\r') && \
		IMUX_WINTEST_PROFILE=$$(wslpath -u "$$wintemp")/imux-wintest.$$$$ && export IMUX_WINTEST_PROFILE && \
		trap 'rm -rf "$$IMUX_WINTEST_PROFILE"' EXIT && \
		GOOS=windows $(GO) test -count=1 -exec "$(CURDIR)/scripts/wsl-exec.sh" $(PKGS); \
	else \
		echo "test-windows: not WSL with Windows interop: Windows tests NOT run, vetting the Windows build only"; \
		GOOS=windows $(GO) vet ./cmd/... ./internal/...; \
	fi

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
