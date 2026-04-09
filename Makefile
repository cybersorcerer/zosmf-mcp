BINARY    := zosmf-mcp
MODULE    := gitlab.com/ronaldfunk/zosmf-mcp
BINARY_DIR := bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS   := -trimpath

.PHONY: all build build-all build-local clean test vet fmt lint run install

all: fmt vet test build-local

build-all:
	@mkdir -p $(BINARY_DIR)
	GOARCH=amd64  GOOS=darwin  go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-darwin-amd64   ./cmd/zosmf-mcp
	GOARCH=arm64  GOOS=darwin  go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-darwin-arm64   ./cmd/zosmf-mcp
	GOARCH=amd64  GOOS=linux   go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-linux-amd64    ./cmd/zosmf-mcp
	GOARCH=s390x  GOOS=linux   go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-linux-s390x    ./cmd/zosmf-mcp
	GOARCH=amd64  GOOS=windows go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-windows.exe   ./cmd/zosmf-mcp

build-local:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/zosmf-mcp

build: build-local

clean:
	rm -f $(BINARY)
	go clean -cache -testcache

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...

run: build
	./$(BINARY) --config .zosmf-mcp-config.yaml

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

tidy:
	go mod tidy

help:
	@echo "Targets:"
	@echo "  all       - fmt, vet, test, build (default)"
	@echo "  build     - Build the binary"
	@echo "  clean     - Remove binary and caches"
	@echo "  test      - Run tests with race detector"
	@echo "  vet       - Run go vet"
	@echo "  fmt       - Format source code"
	@echo "  lint      - Run golangci-lint"
	@echo "  run       - Build and run with default config"
	@echo "  install   - Install binary to ~/.local/bin"
	@echo "  tidy      - Run go mod tidy"
