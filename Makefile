# Nexus Protocol — Cross-Platform Build System
# V10: "The Tauri HUD (The Dashboard)"
# CGO_ENABLED=0 is MANDATORY for static binary with zero C dependencies.

VERSION ?= 0.17.0
BINARY   = nexus
LDFLAGS  = -ldflags "-s -w -X main.nexusVersion=$(VERSION)"

.PHONY: build build-linux build-linux-arm64 build-windows build-windows-arm64 build-all \
        clean lint test test-race test-coverage fmt vet check license-check \
        sidecar sidecar-linux sidecar-linux-arm64 sidecar-windows sidecar-windows-arm64 \
        dashboard-build tauri-deps-install

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

# ─── Tauri Sidecar (V10) ────────────────────────────────────────────────────
# Builds the Go engine as a Tauri 2 sidecar binary for the dashboard.
# Tauri 2 requires binaries at: dashboard/src-tauri/binaries/<name>-<target-triple>
# The base name "nexus" matches `externalBin: ["binaries/nexus"]` in tauri.conf.json.

SIDECAR_DIR = dashboard/src-tauri/binaries

sidecar: sidecar-linux

sidecar-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(SIDECAR_DIR)/nexus-x86_64-unknown-linux-gnu ./cmd/nexus/
	@echo "Built sidecar: $(SIDECAR_DIR)/nexus-x86_64-unknown-linux-gnu"

sidecar-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(SIDECAR_DIR)/nexus-aarch64-unknown-linux-gnu ./cmd/nexus/
	@echo "Built sidecar: $(SIDECAR_DIR)/nexus-aarch64-unknown-linux-gnu"

sidecar-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(SIDECAR_DIR)/nexus-x86_64-pc-windows-msvc.exe ./cmd/nexus/
	@echo "Built sidecar: $(SIDECAR_DIR)/nexus-x86_64-pc-windows-msvc.exe"

sidecar-windows-arm64:
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(SIDECAR_DIR)/nexus-aarch64-pc-windows-msvc.exe ./cmd/nexus/
	@echo "Built sidecar: $(SIDECAR_DIR)/nexus-aarch64-pc-windows-msvc.exe"

sidecar-all: sidecar-linux sidecar-linux-arm64 sidecar-windows sidecar-windows-arm64

# ─── Tauri Dashboard (V10) ──────────────────────────────────────────────────
# Install Tauri build dependencies on Debian/Ubuntu. Run once per CI runner.

tauri-deps-install:
	@echo "Installing Tauri 2 system dependencies..."
	@sudo apt-get update -qq
	@sudo apt-get install -y -qq libwebkit2gtk-4.1-dev libssl-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev 2>/dev/null || \
	  echo "Note: Tauri deps require sudo; on CI use 'sudo: required' or pre-installed image"

dashboard-build: sidecar-linux
	cd dashboard/src-tauri && cargo build --release
	@echo "Tauri dashboard built: dashboard/src-tauri/target/release/nexus-dashboard"

dashboard-build-all: sidecar-all
	cd dashboard/src-tauri && cargo tauri build
	@echo "Tauri bundles built:"
	@find dashboard/src-tauri/target/release/bundle -type f \( -name "*.deb" -o -name "*.AppImage" -o -name "*.msi" -o -name "*.exe" \) 2>/dev/null

# ─── Quality gates ──────────────────────────────────────────────────────────
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

# Coverage ratchet — enforces project's per-package coverage thresholds
test-coverage-ratchet:
	./scripts/coverage-ratchet.sh --check

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
	rm -f $(SIDECAR_DIR)/nexus-x86_64-unknown-linux-gnu
	rm -f $(SIDECAR_DIR)/nexus-aarch64-unknown-linux-gnu
	rm -f $(SIDECAR_DIR)/nexus-x86_64-pc-windows-msvc.exe
	rm -f $(SIDECAR_DIR)/nexus-aarch64-pc-windows-msvc.exe
