# ── Configuration ─────────────────────────────────────────────────────────────

REDOCLY   = npx @redocly/cli@latest
DOCS_DIR  = docs
OPENAPI   = $(DOCS_DIR)/openapi.yaml
CONFIG    = $(DOCS_DIR)/.redocly.yaml

BACKEND_DIR = backend
GO          = go
DC          = docker compose -f $(BACKEND_DIR)/docker-compose.yml

# ── Phony targets ─────────────────────────────────────────────────────────────

.PHONY: help \
        docs docs-lint docs-bundle docs-build \
        backend-build backend-test backend-test-integration backend-lint

# ── Default ───────────────────────────────────────────────────────────────────

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  %-24s %s\n", $$1, $$2}'

# ── Docs ──────────────────────────────────────────────────────────────────────

docs-lint: ## Validate OpenAPI spec
	$(REDOCLY) lint $(OPENAPI) --config $(CONFIG)

docs-bundle: ## Bundle $ref into a single YAML file
	$(REDOCLY) bundle $(OPENAPI) -o $(DOCS_DIR)/openapi.bundle.yaml --config $(CONFIG)

docs-build: ## Build standalone HTML docs
	$(REDOCLY) build-docs $(OPENAPI) -o $(DOCS_DIR)/openapi.html --config $(CONFIG)

docs: docs-lint docs-bundle docs-build ## Lint, bundle, and build HTML docs

# ── Backend ───────────────────────────────────────────────────────────────────

backend-build: ## Compile the backend binary
	cd $(BACKEND_DIR) && $(GO) build ./...

backend-lint: ## Run golangci-lint on the backend
	cd $(BACKEND_DIR) && golangci-lint run --timeout=5m

backend-test: ## Run unit tests with race detector
	cd $(BACKEND_DIR) && $(GO) test -v -race ./...

backend-test-integration: ## Run unit + integration tests with race detector
	cd $(BACKEND_DIR) && $(GO) test -v -race -tags integration ./...

# ── Docker ────────────────────────────────────────────────────────────────────
 
dev-infra: ## Start only infrastructure (postgres, redis, minio) — run app locally
	$(DC) up -d postgres redis minio
 
dev-up: ## Build and start all services (including app container)
	$(DC) up -d --build
 
dev-down: ## Stop and remove all containers (data volumes preserved)
	$(DC) down
 
dev-logs: ## Follow logs for all services (Ctrl+C to stop)
	$(DC) logs -f
 
dev-restart: ## Rebuild and restart the app container only
	$(DC) up -d --build app
 
dev: dev-infra ## Alias for dev-infra (start infra, run app locally)