---
name: update-tools
description: Use when updating a misc Go tool dependency to a new version
---
# Update Tools Skill

Follow these steps to update an existing tool in `misc/`.

## Tool List

| Tool Modfile           | Binary          | Purpose                                  |
| ---------------------- | --------------- | ---------------------------------------- |
| `go-arch-lint-go.mod`  | `go-arch-lint`  | Architecture constraints                 |
| `gofumpt-go.mod`       | `gofumpt`       | Go formatting                            |
| `golangci-lint-go.mod` | `golangci-lint` | Go static analysis (90+ enabled linters) |
| `gopls-go.mod`         | `gopls`         | Go language server / MCP                 |
| `mdsmith-go.mod`       | `mdsmith`       | Markdown linting                         |
| `searxng-mcp-go.mod`   | `searxng-mcp`   | SearXNG MCP server                       |

## Update a Specific Tool

Bump the version in the modfile, then download to update `go.sum`:

```bash
# Edit misc/<tool>-go.mod: change the version line for the main module
# Example: github.com/WinPooh32/searxng-mcp v0.0.2 -> v0.0.3

# Download to update go.sum
go mod download -modfile=misc/<tool>-go.mod
```

Or use `go get` to bump directly:

```bash
go get -tool -modfile=misc/<tool>-go.mod <module>@<version>
```

For latest:

```bash
go get -tool -modfile=misc/<tool>-go.mod <module>@latest
```

## Update All Tools

```bash
for mod in misc/*-go.mod; do
    go get -tool -modfile="$mod" -u
done
```

## Verify

1. Run `make install/tools` to confirm all dependencies download.
2. Run the relevant `make` target to confirm the tool works:
  - `make lint` for linters
  - `make fmt` for formatters
  - `make run/mcp-<tool>` for MCP servers
