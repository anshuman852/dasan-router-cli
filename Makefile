BINARY     := dasan
CMD_DIR    := ./cmd/$(BINARY)
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS    := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
DOCKER_IMG := ghcr.io/anshuman852/dasan

.DEFAULT_GOAL := build

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

.PHONY: build
build: ## Build the binary
	go build -o $(BINARY) $(CMD_DIR)

.PHONY: run
run: build ## Build and run the exporter
	DASAN_HOST=$${DASAN_HOST:-192.168.1.1} \
	DASAN_USERNAME=$${DASAN_USERNAME:-admin} \
	DASAN_PASSWORD=$${DASAN_PASSWORD:?set DASAN_PASSWORD} \
	./$(BINARY) serve

.PHONY: test
test: ## Run tests
	go test ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: vet ## Lint the codebase

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe

# ---------------------------------------------------------------------------
# Release build (matching goreleaser ldflags)
# ---------------------------------------------------------------------------

.PHONY: release-build
release-build: ## Build with version info injected
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD_DIR)
	./$(BINARY) version

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

.PHONY: docker-build
docker-build: ## Build the Docker image
	docker buildx build --platform linux/amd64 --load -t $(DOCKER_IMG):latest .

.PHONY: docker-build-multi
docker-build-multi: ## Build multi-arch Docker image (requires buildx)
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMG):latest .

.PHONY: docker-run
docker-run: ## Run the Docker container
	docker run -d --name dasan-exporter \
		-e DASAN_HOST=$${DASAN_HOST:-192.168.1.1} \
		-e DASAN_USERNAME=$${DASAN_USERNAME:-admin} \
		-e DASAN_PASSWORD=$${DASAN_PASSWORD:?set DASAN_PASSWORD} \
		-p 9800:9800 \
		$(DOCKER_IMG):latest

.PHONY: docker-stop
docker-stop: ## Stop and remove the Docker container
	docker rm -f dasan-exporter 2>/dev/null || true

# ---------------------------------------------------------------------------
# Docker Compose
# ---------------------------------------------------------------------------

.PHONY: compose-up
compose-up: ## Start the exporter
	docker compose up -d dasan

.PHONY: compose-up-monitoring
compose-up-monitoring: ## Start exporter + Prometheus + Grafana
	docker compose --profile monitoring up -d

.PHONY: compose-down
compose-down: ## Stop all services
	docker compose --profile monitoring down

.PHONY: compose-logs
compose-logs: ## Tail logs
	docker compose logs -f dasan

# ---------------------------------------------------------------------------
# CI
# ---------------------------------------------------------------------------

.PHONY: ci
ci: tidy lint test build ## Run full CI pipeline locally

# ---------------------------------------------------------------------------
# Misc
# ---------------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
