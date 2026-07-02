# Vault Sync Makefile

.PHONY: build clean test test-integration install dev-deps lint fmt vet release-local

# Variables
BINARY_NAME=vaultsync
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -w -s

# Default target
all: build

# Build the application
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/vaultsync

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64 ./cmd/vaultsync
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux-arm64 ./cmd/vaultsync
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-darwin-amd64 ./cmd/vaultsync
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-darwin-arm64 ./cmd/vaultsync
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-windows-amd64.exe ./cmd/vaultsync

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

# Run tests
test:
	go test -v ./...

# Run integration tests against a live Vault (see vault_integration_test.go).
# Requires VAULT_ADDR/VAULT_TOKEN pointing at a reachable Vault.
test-integration:
	VAULTSYNC_INTEGRATION=1 go test -run Integration -v ./...

# Install the binary
install: build
	sudo mv $(BINARY_NAME) /usr/local/bin/

# Development dependencies
dev-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Lint the code
lint:
	golangci-lint run

# Format the code
fmt:
	go fmt ./...

# Vet the code
vet:
	go vet ./...

# Run all checks
check: fmt vet lint test

# Create a local release (for testing)
release-local: clean build-all
	mkdir -p dist
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64 README.adoc LICENSE
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz $(BINARY_NAME)-linux-arm64 README.adoc LICENSE
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64 README.adoc LICENSE
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64 README.adoc LICENSE
	zip dist/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe README.adoc LICENSE
	cd dist && sha256sum * > checksums.txt

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  build-all    - Build for all platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-integration - Run integration tests against a live Vault"
	@echo "  install      - Install the binary to /usr/local/bin"
	@echo "  dev-deps     - Install development dependencies"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  check        - Run fmt, vet, lint, and test"
	@echo "  release-local - Create local release archives"
	@echo "  help         - Show this help message"