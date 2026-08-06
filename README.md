# singularity

A simple AI-driven project built with Go.

This project uses AI assistance for development. It demonstrates how AI can
scaffold and build software from scratch.

## Getting Started

### Prerequisites

- Go 1.26+
- curl, jq (for tool installation)
- [SearXNG](https://docs.searxng.org/admin/installation.html) — self-hosted search
  engine (required for `run/mcp-searxng`)

### Initialize

```bash
# Download all tool dependencies
make install/tools

# Verify setup
make lint
```

## License

MIT
