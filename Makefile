# Nexus Protocol — Cross-Platform Build System
# V6: "The Global Open Source Release"
# CGO_ENABLED=0 is MANDATORY for static binary with zero C dependencies.

VERSION ?= 0.6.0
BINARY   = nexus
LDFLAGS  = -ldflags "-s -w -X main.nexusVersion=$(VERSION)"

.PHONY: build build-linux build-linux-arm64 build-windows build-windows-arm64 build-all \
        clean lint test test-race test-coverage fmt vet check license-check

# Default: build for current platform
build:
        CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/nexus/

# Cross-compilation targets
build-linux:
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/nexus/

build-linux-arm64:
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 ./cmd/nexus/

build-windows:
        CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-windows-amd64.exe ./cmd/nexus/

build-windows-arm64:
        CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-windows-arm64.exe ./cmd/nexus/

build-all: build-linux build-linux-arm64 build-windows build-windows-arm64

# Quality gates
# lint uses golangci-lint (matches CI pipeline). Falls back to go vet if
# golangci-lint is not installed, with a warning to install it.
lint:
        @if command -v golangci-lint >/dev/null 2>&1; then \
                golangci-lint run --timeout=5m ./...; \
        else \
                echo "⚠️  golangci-lint not found, falling back to 'go vet'"; \
                echo "   Install: https://golangci-lint.run/usage/install/"; \
                go vet ./...; \
        fi

test:
        go test ./...

test-race:
        go test -race ./...

test-coverage:
        go test -coverprofile=coverage.out ./...
        go tool cover -html=coverage.out -o coverage.html
        @echo "Coverage report: coverage.html"

fmt:
        gofmt -w .
        goimports -w .

vet:
        go vet ./...

# Full check (run before commits — mirrors CI: fmt, lint, test)
check: fmt lint test
        @echo "All checks passed."

# License header check (mirrors CI: license-headers job)
license-check:
        @bash scripts/check-license-headers.sh
        @echo "License headers OK."

# Clean build artifacts
clean:
        rm -f $(BINARY) $(BINARY)-linux-amd64 $(BINARY)-linux-arm64 $(BINARY)-windows-amd64.exe $(BINARY)-windows-arm64.exe
        rm -f coverage.out coverage.html
        rm -rf dist/
