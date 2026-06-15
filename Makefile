SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help setup run dev build test lint pre-commit pre-commit-fast \
        generate db-up db-reset tailwind-install js-vendor clean \
        run-op dev-op generate-sqlc generate-templ css-build css-watch \
        semgrep deadcode db-start db-down db-down-clean db-logs db-psql \
        db-reset-pgadmin db-migrate-create loadtest-browse loadtest-rate-burst \
        db-wait \
        dev-seed db-reset-seed codeql

# ─────────────────────────────────────────────────────────────────────
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } \
	/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Common

setup: ## One-time setup: install toolchain via mise, configure git hooks
	mise install
	git config core.hooksPath githooks

run: ## Start dev server with live reload (uses 1Password CLI if available)
	@if command -v op >/dev/null 2>&1; then \
	  echo "→ using 1Password CLI for env injection"; \
	  op run --env-file=.env -- air; \
	else \
	  echo "→ using .env directly (no 1Password CLI found)"; \
	  air; \
	fi

dev: ## Tailwind watch + dev server in parallel
	bin/tailwindcss -i cmd/server/public/css/input.css -o cmd/server/public/css/output.css --watch & \
	$(MAKE) run; \
	kill %1 2>/dev/null || true

test: ## Run race-enabled tests
	go test -race -count=1 -timeout 120s ./...

coverage: ## Coverage report excluding sqlc/templ generated code
	go test -coverprofile=coverage.out -count=1 -timeout 120s ./...
	@grep -v '_templ\.go' coverage.out \
		| grep -v 'vigil/internal/db/' \
		| grep -v 'vigil/internal/testutil/' \
		> coverage-filtered.out
	@echo "── Coverage (excluding generated code) ──────────────────────────────"
	@go tool cover -func=coverage-filtered.out | grep -v ' 100\.0%' | grep -v 'stub_querier' | tail -30
	@echo ""
	@go tool cover -func=coverage-filtered.out | tail -1
	@rm -f coverage.out coverage-filtered.out

pre-commit-fast: generate ## Fast pre-commit suite (lint, govulncheck, tests)
	go mod tidy
	golangci-lint run ./...
	govulncheck ./...
	go test -race -count=1 -timeout 120s ./...

pre-commit: pre-commit-fast semgrep ## Full pre-commit suite (includes semgrep)

##@ Build & generate

build: ## Build server binary to bin/server
	@mkdir -p bin
	go build -o bin/server ./cmd/server

generate: generate-sqlc generate-templ css-build js-vendor ## Run all code generation

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format code (gofmt + goimports via golangci-lint)
	golangci-lint fmt

clean: ## Remove build artifacts
	rm -rf bin/

##@ Database

db-up: ## Start the local PostgreSQL container
	podman compose up -d

db-reset: ## Reset the local database (wipes data)
	podman compose down -v
	podman compose up -d

db-reset-seed: db-reset ## Reset DB and run dev seed (development only)
	$(MAKE) db-wait
	$(MAKE) dev-seed APP_ENV=development DEV_SEED=true

db-wait: ## Wait until local PostgreSQL accepts connections
	@echo "→ waiting for postgres to accept connections"
	@for i in $$(seq 1 60); do \
		if podman compose exec -T postgres pg_isready -U vigil -d vigil >/dev/null 2>&1; then \
			echo "→ postgres is ready"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "ERROR: postgres did not become ready in time"; \
	exit 1

dev-seed: ## Seed coherent development data (manual, not CI)
	APP_ENV=development DEV_SEED=true go run ./cmd/seed

##@ Developer tools (manual / occasional)

loadtest-browse: ## Run ad-hoc k6 read-path scenario (manual, not CI)
	@command -v k6 >/dev/null 2>&1 || (echo "k6 not found. Install with: brew install k6"; exit 1)
	@BASE_URL=$${BASE_URL:-http://localhost:8080} \
	EXPECT_RATE_LIMIT=$${EXPECT_RATE_LIMIT:-false} \
	k6 run scripts/loadtest/baseline-read.js

loadtest-rate-burst: ## Run ad-hoc k6 burst scenario to observe 429 behavior
	@command -v k6 >/dev/null 2>&1 || (echo "k6 not found. Install with: brew install k6"; exit 1)
	@BASE_URL=$${BASE_URL:-http://localhost:8080} k6 run scripts/loadtest/rate-limit-burst.js

##@ One-time setup

js-vendor: ## Install browser JS deps (bun) and copy into the embed dir
	bun install
	@mkdir -p cmd/server/public/js
	cp node_modules/htmx.org/dist/htmx.min.js     cmd/server/public/js/htmx.min.js
	cp node_modules/@alpinejs/csp/dist/cdn.min.js cmd/server/public/js/alpine.min.js
	cp node_modules/mermaid/dist/mermaid.min.js   cmd/server/public/js/mermaid.min.js

# ── Maintainer / hidden targets (no help text — won't appear in `make help`) ──

run-op:
	op run --env-file=.env -- air

dev-op:
	bin/tailwindcss -i cmd/server/public/css/input.css -o cmd/server/public/css/output.css --watch & \
	op run --env-file=.env -- air; \
	kill %1 2>/dev/null || true

generate-sqlc:
	sqlc generate

generate-templ:
	templ generate

css-build:
	bin/tailwindcss -i cmd/server/public/css/input.css -o cmd/server/public/css/output.css --minify

css-watch:
	bin/tailwindcss -i cmd/server/public/css/input.css -o cmd/server/public/css/output.css --watch

semgrep:
	semgrep scan --error \
		--config "p/golang" \
		--config "p/secrets" \
		--config "p/owasp-top-ten" \
		--config "p/supply-chain"

codeql:
	codeql database create vigil-db --language=go --overwrite --source-root=.
	codeql database analyze vigil-db \
		codeql/go-queries:codeql-suites/go-security-extended.qls \
		--format=sarif-latest --output=vigil-results.sarif --sarif-category=go --threads=0
	@echo "Åpne vigil-results.sarif i SARIF Viewer"

deadcode:
	deadcode -test ./...
