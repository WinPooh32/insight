-include .env

.PHONY: help lint lint/ci lint/go lint/typos lint/md lint/arch lint/bd lint/untested fmt fmt/go fmt/golangci-lint fmt/md run/mcp-gopls run/mcp-searxng run/mcp-lightpanda run/lightpanda/fetch run/lightpanda/serve install/tools test test/unit test/all test/mutesting test/mutest-pkg test/coverage gen/insight-storage run/insight build/insight bd/prime bd/ready bd/claim bd/create bd/show bd/create bd/close bd/search

## Show available targets
help:
	@grep -E '^[a-zA-Z_/-]+:.*## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

## Run all linters
lint: lint/go lint/typos lint/md lint/arch lint/bd lint/untested

## Run ci linters
lint/ci: lint/go lint/typos lint/md lint/arch lint/untested

lint/go:
	@echo "Run go linter"
	@go tool -modfile=misc/golangci-lint-go.mod golangci-lint run ./... -c .golangci.yml

lint/typos:
	@echo "Run typos linter"
	@misc/bin/typos

lint/md:
	@echo "Run markdown linter"
	@go tool -modfile=misc/mdsmith-go.mod mdsmith check

lint/arch:
	@echo "Run architecture linter"
	@go tool -modfile=misc/go-arch-lint-go.mod go-arch-lint check

lint/bd:
	@echo "Run beads linter"
	@misc/bin/bd lint

## Check for packages without tests
lint/untested:
	@echo "Check for packages without tests"
	@misc/scripts/find-untested-pkgs.sh

## Format all source files
fmt: fmt/go fmt/golangci-lint fmt/md

fmt/go:
	@echo "Format Go source files"
	@go tool -modfile=misc/gofumpt-go.mod gofumpt -l -w .

fmt/golangci-lint:
	@echo "Auto-fix lint issues"
	@go tool -modfile=misc/golangci-lint-go.mod golangci-lint run ./... -c .golangci.yml --fix --timeout=5m --issues-exit-code=0

fmt/md:
	@echo "Format Markdown files"
	@go tool -modfile=misc/mdsmith-go.mod mdsmith fix || true

## Run MCP servers
run/mcp-gopls:
	@echo "Run gopls MCP server"
	@go tool -modfile=misc/gopls-go.mod gopls mcp

run/mcp-searxng:
	@echo "Run SearXNG MCP server"
	@go tool -modfile=misc/searxng-mcp-go.mod searxng-mcp $(SEARXNG_ARGS)

run/mcp-lightpanda:
	@echo "Run lightpanda MCP server"
	@misc/bin/lightpanda mcp $(LIGHTPANDA_ARGS)

## Run beads
bd/prime:
	@misc/bin/bd prime

# Show issues ready to work on
bd/ready:
	@misc/bin/bd ready

# Claim issue
bd/claim:
	@misc/bin/bd update $(TASK_ID) --claim

# Review issue details
bd/show:
	@misc/bin/bd show $(TASK_ID)

# Create issues
bd/create:
	@misc/bin/bd --title $(TITLE) --description $(DESCRIPTION) --type=$(TYPE)

# Close issues
bd/close:
	@misc/bin/bd close $(ISSUES) --reason $(REASON)

# Search issues by keyword
bd/search:
	@misc/bin/bd search ${QUERY}

install/tools:
	@echo "Downloading misc tool dependencies..."
	@for mod in misc/*-go.mod; do \
		echo "Processing $$mod..."; \
		go mod download -modfile="$$mod"; \
	done
	@echo "Installing typos..."
	@bash misc/scripts/install-typos.sh
	@echo "Installing beads..."
	@bash misc/scripts/install-beads.sh
	@echo "Installing lightpanda..."
	@bash misc/scripts/install-lightpanda.sh

## Run all tests
test: test/all

test/unit:
	@echo "Run unit tests"
	@go tool -modfile=misc/gotestsum-go.mod gotestsum --format-hide-empty-pkg --format=pkgname-and-test-fails --format-icons=hivis -- -short -timeout=60s ./...

test/all:
	@echo "Run all tests"
	@go tool -modfile=misc/gotestsum-go.mod gotestsum --format-hide-empty-pkg --format=pkgname-and-test-fails --format-icons=hivis -- -timeout=60s ./...

test/mutesting:
	@echo "Run mutation testing"
	@misc/scripts/mutest-tested-pkgs.sh

test/mutest-pkg:
	@echo "Run mutation testing"
	@go tool -modfile=misc/go-mutesting-go.mod go-mutesting --exec=misc/scripts/mutate-test.sh --config .mutesting.yml $(PACKAGE)

## Coverage targets
test/coverage:
	@echo "Run tests with coverage"
	@go tool -modfile=misc/gotestsum-go.mod gotestsum --format-hide-empty-pkg --format=pkgname-and-test-fails --format-icons=hivis -- -timeout=60s -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -n 1

## Service targets
gen/insight-storage:
	@echo "Generate sqlc code"
	@go tool -modfile=misc/sqlc-go.mod sqlc generate -f cmd/insight/internal/storage/sqlc.yaml

run/insight:
	@echo "Run insight service"
	@go run ./cmd/insight

## Build relay service
build/insight:
	@echo "Build insight service"
	@go build -o build/insight ./cmd/insight
