.PHONY: build clean run dev release

VERSION := 0.1.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Default target
all: build

# Build web assets with Vite
web:
	cd web && npm install && npm run build

# Build the binary
build: web
	go build $(LDFLAGS) -o build/hugo-manager ./cmd/hugo-manager

# Build with smaller binary size
build-small: web
	go build $(LDFLAGS) -ldflags "-s -w" -o build/hugo-manager ./cmd/hugo-manager

# Build Go only (skip web build)
build-go:
	go build $(LDFLAGS) -o build/hugo-manager ./cmd/hugo-manager

# Run in development
run:
	go run ./cmd/hugo-manager --dir $(or $(DIR),.)

# Clean build artifacts
clean:
	rm -rf build/
	rm -rf dist/
	rm -rf web/dist/
	rm -rf web/node_modules/

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run tests
test:
	go test -v ./...

# Build for multiple platforms
release: clean
	mkdir -p dist
	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/hugo-manager-linux-amd64 ./cmd/hugo-manager
	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/hugo-manager-linux-arm64 ./cmd/hugo-manager
	# macOS AMD64
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/hugo-manager-darwin-amd64 ./cmd/hugo-manager
	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/hugo-manager-darwin-arm64 ./cmd/hugo-manager
	# Windows AMD64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/hugo-manager-windows-amd64.exe ./cmd/hugo-manager
	# Create archives
	cd dist && tar -czf hugo-manager-$(VERSION)-linux-amd64.tar.gz hugo-manager-linux-amd64
	cd dist && tar -czf hugo-manager-$(VERSION)-linux-arm64.tar.gz hugo-manager-linux-arm64
	cd dist && tar -czf hugo-manager-$(VERSION)-darwin-amd64.tar.gz hugo-manager-darwin-amd64
	cd dist && tar -czf hugo-manager-$(VERSION)-darwin-arm64.tar.gz hugo-manager-darwin-arm64
	cd dist && zip hugo-manager-$(VERSION)-windows-amd64.zip hugo-manager-windows-amd64.exe

# Install locally
install: build
	mv build/hugo-manager $(GOPATH)/bin/

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Show help
help:
	@echo "Hugo Manager Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build       - Build the binary"
	@echo "  build-small - Build with smaller binary size"
	@echo "  run         - Run in development (use DIR=path to specify project)"
	@echo "  clean       - Remove build artifacts"
	@echo "  deps        - Download and tidy dependencies"
	@echo "  test        - Run tests"
	@echo "  release     - Build for all platforms"
	@echo "  install     - Install to GOPATH/bin"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run DIR=/path/to/hugo/site"
	@echo "  make release"
