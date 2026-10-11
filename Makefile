BINARY    := zosmf-mcp
MODULE    := github.com/cybersorcerer/zosmf-mcp
BINARY_DIR := bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS   := -trimpath

# Keep in step with the version in .github/workflows/build.yml
GOLANGCI_VERSION ?= v2.12.0

# make deploy updates the container on another machine; REMOTE_HOST,
# REMOTE_USER and REMOTE_REPO (the repository checkout there) come from .env
-include .env
REMOTE_HOST ?=
REMOTE_USER ?=
REMOTE_REPO ?=
REMOTE      := $(REMOTE_USER)@$(REMOTE_HOST)

# A binary here replaces the one built into the container image. make deploy
# puts it on the target machine; delete it to go back to the image's build.
CONTAINER_BINARY := $(BINARY_DIR)/$(BINARY)-container

.PHONY: all build build-all build-local clean test vet fmt lint check-identifiers run install deploy apikey docker-build docker-up docker-up-direct docker-down docker-logs container-build container-up container-up-direct container-down container-logs launch-agent launch-agent-remove install-skill

all: fmt vet check-identifiers test build-local

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

# Pinned to the version the CI lint job uses, and run through `go run` so a
# local install is not required and cannot drift from CI.
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./...

check-identifiers:
	@./scripts/check-identifiers.sh

run: build
	./$(BINARY_DIR)/$(BINARY) --config .zosmf-mcp-config.yaml

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY_DIR)/$(BINARY) $(HOME)/.local/bin/$(BINARY)

deploy:
	@test -n "$(REMOTE_HOST)" -a -n "$(REMOTE_USER)" -a -n "$(REMOTE_REPO)" || { echo "REMOTE_HOST, REMOTE_USER and REMOTE_REPO must be set — put them in .env or pass them on the command line"; exit 1; }
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_DIR)/$(BINARY)-linux-arm64 ./cmd/zosmf-mcp
	ssh $(REMOTE) 'mkdir -p $(REMOTE_REPO)/$(BINARY_DIR)'
	scp $(BINARY_DIR)/$(BINARY)-linux-arm64 $(REMOTE):$(REMOTE_REPO)/$(CONTAINER_BINARY)
	# container is in /usr/local/bin, which a non-interactive SSH session lacks on its PATH
	ssh $(REMOTE) 'cd $(REMOTE_REPO) && PATH=/usr/local/bin:$$PATH make container-down container-up-direct'
	ssh $(REMOTE) 'PATH=/usr/local/bin:$$PATH container exec zosmf-mcp /usr/local/bin/zosmf-mcp --version'

tidy:
	go mod tidy

apikey:
	@openssl rand -base64 32

docker-build:
	VERSION=$(VERSION) COMMIT=$(COMMIT) docker compose build

# With the jump host tunnel — for a host that cannot reach z/OSMF itself.
docker-up: docker-build
	docker compose --profile tunnel up -d

# Without it — for a host that can.
docker-up-direct: docker-build
	docker compose up -d

docker-down:
	docker compose --profile tunnel down

docker-logs:
	docker compose logs -f

# --- Apple container (macOS, Apple Silicon) -------------------------------
# Apple's `container` has no compose equivalent, so the two containers are
# started by hand and the tunnel's IP is resolved at runtime.
CONTAINER_IMAGE ?= zosmf-mcp:latest

# container build starts Apple's BuildKit builder (2 CPUs, 2 GB) and leaves it
# running. Stop it again afterwards, unless it was running before the build.
container-build:
	@BUILDER_WAS_RUNNING=$$(container builder status 2>/dev/null | awk 'NR > 1 && $$3 == "running"' | wc -l); \
	echo "container build --platform linux/arm64 -t $(CONTAINER_IMAGE) ."; \
	container build --platform linux/arm64 \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	  -t $(CONTAINER_IMAGE) .; \
	RC=$$?; \
	if [ "$$BUILDER_WAS_RUNNING" -eq 0 ]; then container builder stop >/dev/null 2>&1 && echo "builder stopped again"; fi; \
	exit $$RC

container-up: container-build
	@test -f .env || { echo ".env is missing — run: cp .env.example .env and fill it in"; exit 1; }
	@set -a; . ./.env; set +a; \
	test -n "$$JUMP_HOST" || { echo "JUMP_HOST is not set"; exit 1; }; \
	test -n "$$MCP_API_KEY" || { echo "MCP_API_KEY is not set — generate one with: make apikey"; exit 1; }; \
	container run -d --name zosmf-tunnel \
	  -v "$${JUMP_KEY:-./secrets/jump_key}:/key" \
	  --entrypoint /usr/bin/ssh $(CONTAINER_IMAGE) \
	  -N -D 0.0.0.0:1080 -i /key \
	  -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
	  -o ServerAliveCountMax=3 -o StrictHostKeyChecking=accept-new \
	  -o BatchMode=yes "$$JUMP_HOST" >/dev/null; \
	sleep 3; \
	TUNNEL_IP=$$(container ls | awk '/^zosmf-tunnel/{print $$6}' | cut -d/ -f1); \
	test -n "$$TUNNEL_IP" || { echo "tunnel container is not running — check: container logs zosmf-tunnel"; exit 1; }; \
	echo "tunnel listening on $$TUNNEL_IP:1080"; \
	ENVFILE=$$(mktemp -t zosmf-mcp-env); chmod 600 "$$ENVFILE"; \
	trap 'rm -f "$$ENVFILE"' EXIT INT TERM; \
	sed -E 's/^[[:space:]]*export[[:space:]]+//' .env | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' > "$$ENVFILE"; \
	BINMOUNT=""; \
	[ -f $(CONTAINER_BINARY) ] && { BINMOUNT="-v $$PWD/$(CONTAINER_BINARY):/usr/local/bin/zosmf-mcp"; echo "using $(CONTAINER_BINARY) instead of the image's build"; }; \
	container run -d --name zosmf-mcp \
	  -v "$${ZOSMF_CONFIG:-./.smpe-zosmf.yaml}:/config/.smpe-zosmf.yaml" \
	  -v "$${SSH_KEY_DIR:-./secrets}:/keys" \
	  $$BINMOUNT \
	  --env-file "$$ENVFILE" \
	  -e MCP_API_KEY="$$MCP_API_KEY" \
	  -e ZOSMF_PROXY=socks5://$$TUNNEL_IP:1080 \
	  -p 8081:8081 \
	  $(CONTAINER_IMAGE) --transport streamable-http >/dev/null; \
	echo "MCP server listening on http://localhost:8081/mcp"

# On a host that reaches z/OSMF itself there is nothing to tunnel: one
# container, no SOCKS proxy. ZOSMF_PROXY is filtered out of the environment
# so a .env kept for the tunnel setup cannot send traffic nowhere.
container-up-direct: container-build
	@test -f .env || { echo ".env is missing — run: cp .env.example .env and fill it in"; exit 1; }
	@set -a; . ./.env; set +a; \
	test -n "$$MCP_API_KEY" || { echo "MCP_API_KEY is not set — generate one with: make apikey"; exit 1; }; \
	ENVFILE=$$(mktemp -t zosmf-mcp-env); chmod 600 "$$ENVFILE"; \
	trap 'rm -f "$$ENVFILE"' EXIT INT TERM; \
	sed -E 's/^[[:space:]]*export[[:space:]]+//' .env | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' \
	  | grep -v '^ZOSMF_PROXY=' > "$$ENVFILE"; \
	KEYS="$${SSH_KEY_DIR:-./secrets}"; KEYMOUNT=""; \
	[ -d "$$KEYS" ] && KEYMOUNT="-v $$KEYS:/keys"; \
	BINMOUNT=""; \
	[ -f $(CONTAINER_BINARY) ] && { BINMOUNT="-v $$PWD/$(CONTAINER_BINARY):/usr/local/bin/zosmf-mcp"; echo "using $(CONTAINER_BINARY) instead of the image's build"; }; \
	container run -d --name zosmf-mcp \
	  -v "$${ZOSMF_CONFIG:-./.smpe-zosmf.yaml}:/config/.smpe-zosmf.yaml" \
	  $$KEYMOUNT \
	  $$BINMOUNT \
	  --env-file "$$ENVFILE" \
	  -e MCP_API_KEY="$$MCP_API_KEY" \
	  -p 8081:8081 \
	  $(CONTAINER_IMAGE) --transport streamable-http >/dev/null; \
	echo "MCP server listening on http://localhost:8081/mcp"

# Each container is handled on its own: the direct setup has no tunnel, and
# naming a container that does not exist makes `container` exit non-zero for
# the whole call, which make then reports as an ignored error.
container-down:
	@for c in zosmf-mcp zosmf-tunnel; do \
	  container stop $$c >/dev/null 2>&1 || true; \
	  container rm $$c >/dev/null 2>&1 || true; \
	done
	@echo "stopped"

container-logs:
	container logs -f zosmf-mcp

# Autostart at login (macOS). Apple container has no restart policy, so the
# container is brought up by a LaunchAgent instead.
launch-agent:
	@./scripts/install-launch-agent.sh

launch-agent-remove:
	@./scripts/install-launch-agent.sh --uninstall

# Copies the setup instructions into an agent's own directory. Nothing is
# installed anywhere without this being run on purpose.
AGENT ?= claude
SCOPE ?= project

install-skill: build
	@$(BINARY_DIR)/$(BINARY) init skill --agent $(AGENT) --scope $(SCOPE)

help:
	@echo "Targets:"
	@echo "  all       - fmt, vet, test, build (default)"
	@echo "  build     - Build the binary"
	@echo "  clean     - Remove binary and caches"
	@echo "  test      - Run tests with race detector"
	@echo "  vet       - Run go vet"
	@echo "  fmt       - Format source code"
	@echo "  lint      - Run golangci-lint"
	@echo "  check-identifiers - Fail on private IPs, internal hostnames or keys in tracked files"
	@echo "  run       - Build and run with default config"
	@echo "  install   - Install binary to ~/.local/bin"
	@echo "  deploy    - Build a linux/arm64 binary, copy it to REMOTE_REPO on REMOTE_USER@REMOTE_HOST"
	@echo "              and restart the container there with it (REMOTE_* are read from .env)"
	@echo "  tidy      - Run go mod tidy"
	@echo "  apikey    - Generate a bearer token for the HTTP transports"
	@echo "  install-skill - Install the guided-setup instructions for an agent"
	@echo "              AGENT=claude|opencode|pi (default claude), SCOPE=project|user (default project)"
	@echo ""
	@echo "Containers (see docs/container-setup.md):"
	@echo "  docker-build/-up/-down/-logs      - Docker with Compose, server plus jump host tunnel"
	@echo "  docker-up-direct                  - Docker, server only (host reaches z/OSMF itself)"
	@echo "  container-build/-up/-down/-logs   - Apple container, server plus jump host tunnel"
	@echo "  container-up-direct               - Apple container, server only"
	@echo "  launch-agent / launch-agent-remove - Start the container at login (macOS)"
