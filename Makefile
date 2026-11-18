.PHONY: all
all: build

.PHONY: help
help:
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the plugin binary
	@echo "Building CloudZero plugin..."
	@go build -o bin/finfocus-plugin-cloudzero ./cmd/finfocus-plugin-cloudzero

.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running golangci-lint (v2.15.0)..."
	@golangci-lint run ./...

.PHONY: lint-md
lint-md: ## Lint markdown files with markdownlint
	@echo "Running markdownlint..."
	@npm list -g markdownlint-cli2 >/dev/null 2>&1 || npm install -g markdownlint-cli2
	@markdownlint-cli2 "**/*.md" "#node_modules/**"

.PHONY: lint-prose-sync
lint-prose-sync: ## Download/refresh Vale style packages
	@echo "Syncing Vale styles..."
	@vale sync

.PHONY: lint-prose
lint-prose: lint-prose-sync ## Lint prose documentation (markdown)
	@echo "Linting prose..."
	@vale ROADMAP.md CONTEXT.md docs/design.md README.md CLAUDE.md

.PHONY: lint-prose-ci
lint-prose-ci: ## Vale output in reviewdog line format (used by CI)
	@vale --no-exit --output=line ROADMAP.md CONTEXT.md docs/design.md README.md CLAUDE.md

.PHONY: vuln
vuln: ## Check for security vulnerabilities
	@echo "Running govulncheck..."
	@govulncheck ./...

.PHONY: test
test: ## Run all tests
	@echo "Running tests..."
	@go test -v ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test -v -race ./...

.PHONY: cover
cover: ## Fail when coverage is below the plan.md targets (80% overall, 95% client and plugin)
	@echo "Checking coverage..."
	@./scripts/check-coverage.sh

.PHONY: test-integration
test-integration: ## Live CloudZero calls. Skips unless CLOUDZERO_API_KEY is set.
	@echo "Running integration tests..."
	@go test -tags=integration ./test/integration/...

.PHONY: fmt
fmt: ## Format code with gofmt
	@echo "Formatting code..."
	@gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

.PHONY: validate
validate: fmt vet test cover lint vuln ## Run validation (format, vet, test, cover, lint, vuln)

.PHONY: release-check
release-check: ## Verify goreleaser configuration
	@echo "Checking goreleaser configuration..."
	@goreleaser check

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f bin/finfocus-plugin-cloudzero
	@rm -rf dist/

.PHONY: dev
dev: build ## Build and run plugin
	@echo "Running plugin..."
	@./bin/finfocus-plugin-cloudzero --port 9090
