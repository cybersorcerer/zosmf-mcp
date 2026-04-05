BINARY    := zosmf-mcp
MODULE    := gitlab.com/ronaldfunk/zosmf-mcp
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -s -w -X main.version=$(VERSION)
GOFLAGS   := -trimpath

.PHONY: all build clean test vet fmt lint run install

all: fmt vet test build

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/zosmf-mcp

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
