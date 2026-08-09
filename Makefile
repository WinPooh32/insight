-include .env

.PHONY: help lint lint/go lint/typos lint/md lint/arch fmt fmt/go fmt/golangci-lint fmt/md run/mcp-gopls run/mcp-searxng run/mcp-lightpanda run/lightpanda/fetch run/lightpanda/serve install/tools test test/unit test/all test/mutesting gen/insight-storage run/insight build/insight

## Show available targets
help:
	@grep -E '^[a-zA-Z_/-]+:.*## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

## Run all linters
lint: lint/go lint/typos lint/md lint/arch

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

install/tools:
	@echo "Downloading misc tool dependencies..."
	@for mod in misc/*-go.mod; do \
		echo "Processing $$mod..."; \
		go mod download -modfile="$$mod"; \
	done
	@echo "Installing typos..."
	@bash misc/scripts/install-typos.sh
	@echo "Installing lightpanda..."
	@bash misc/scripts/install-lightpanda.sh

## Run all tests
test: test/unit test/all test/mutesting

test/unit:
	@echo "Run unit tests"
	@go tool -modfile=misc/gotestsum-go.mod gotestsum -- -short -timeout=60s ./...

test/all:
	@echo "Run all tests"
	@go tool -modfile=misc/gotestsum-go.mod gotestsum -- -timeout=60s ./...

test/mutesting:
	@echo "Run mutation testing"
	@go tool -modfile=misc/go-mutesting-go.mod go-mutesting --exec=misc/scripts/mutate-test.sh --test-recursive ./...

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
