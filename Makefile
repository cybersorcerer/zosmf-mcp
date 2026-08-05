BINARY    := zosmf-mcp
MODULE    := gitlab.com/ronaldfunk/zosmf-mcp
BINARY_DIR := bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS   := -trimpath

# Remote deployment target (macOS Apple Silicon); REMOTE_HOST/REMOTE_USER come from .env
-include .env
REMOTE_HOST ?=
REMOTE_USER ?=
REMOTE_DIR  ?= ~/.local/bin
REMOTE      := $(REMOTE_USER)@$(REMOTE_HOST)

.PHONY: all build build-all build-local clean test vet fmt lint run install deploy apikey docker-build docker-up docker-down docker-logs

all: fmt vet test build-local

build-all:
	@mkdir -p $(BINARY_DIR)
	GOARCH=amd64  GOOS=darwin  go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-darwin-amd64   ./cmd/zosmf-mcp
	GOARCH=arm64  GOOS=darwin  go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-darwin-arm64   ./cmd/zosmf-mcp
	GOARCH=amd64  GOOS=linux   go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-linux-amd64    ./cmd/zosmf-mcp
	GOARCH=s390x  GOOS=linux   go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-linux-s390x    ./cmd/zosmf-mcp
	GOARCH=amd64  GOOS=windows go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-windows.exe   ./cmd/zosmf-mcp

build-local:
	@mkdir -p $(BINARY_DIR)
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY) ./cmd/zosmf-mcp

build: build-local

clean:
	rm -rf $(BINARY_DIR)
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
	./$(BINARY_DIR)/$(BINARY) --config .zosmf-mcp-config.yaml

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY_DIR)/$(BINARY) $(HOME)/.local/bin/$(BINARY)

deploy:
	@test -n "$(REMOTE_HOST)" -a -n "$(REMOTE_USER)" || { echo "REMOTE_HOST/REMOTE_USER not set — put them in .env or pass on the command line"; exit 1; }
	@mkdir -p $(BINARY_DIR)
	GOARCH=arm64 GOOS=darwin go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-darwin-arm64 ./cmd/zosmf-mcp
	ssh $(REMOTE) 'mkdir -p $(REMOTE_DIR)'
	scp $(BINARY_DIR)/$(BINARY)-darwin-arm64 $(REMOTE):$(REMOTE_DIR)/$(BINARY)
	# macOS rejects the linker-signed ad-hoc signature after transfer — re-sign on the target
	ssh $(REMOTE) 'chmod 755 $(REMOTE_DIR)/$(BINARY) && xattr -c $(REMOTE_DIR)/$(BINARY) && codesign -f -s - $(REMOTE_DIR)/$(BINARY) && $(REMOTE_DIR)/$(BINARY) --version'
	@echo "deployed to $(REMOTE):$(REMOTE_DIR)/$(BINARY) — restart the server there manually"

tidy:
	go mod tidy

apikey:
	@openssl rand -base64 32

docker-build:
	docker compose build

docker-up: docker-build
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

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
	@echo "  deploy    - Cross-compile for macOS arm64 and copy to REMOTE_USER@REMOTE_HOST:~/.local/bin"
	@echo "              REMOTE_HOST/REMOTE_USER are read from .env; override on the command line if needed"
	@echo "  tidy      - Run go mod tidy"
