# Development

Requires Go 1.24+.

## Setup

```bash
git clone https://github.com/factorly-hq/factorly-cli.git
cd factorly
make init       # download deps + install tooling (golangci-lint, gotestsum)
make build      # build for host platform → build/factorly
```

## Make targets

```bash
make init               # download deps + install tooling (golangci-lint, gotestsum)
make build              # build for host platform → build/factorly
make test               # run unit + integration tests
make test-unit          # unit tests only
make test-integration   # integration tests only (builds binary first)
make ci                 # full CI pipeline: tidy, fmt, vet, lint, test
make lint               # run golangci-lint
make fmt                # auto-fix lint issues + format code
make vet                # go vet
make tidy               # go mod tidy
make clean              # remove build artifacts
make version            # bump patch version (BUMP=minor|major)
make release            # cross-platform binaries (linux, darwin, windows)
```

## Project structure

```
src/
├── cmd/factorly/          # CLI entrypoint + commands
├── internal/
│   ├── config/            # YAML config loading + validation
│   ├── provider/          # CLI, REST, MCP providers
│   ├── proxy/             # Proxy engine (route + log)
│   ├── registry/          # Tool registry
│   ├── server/            # MCP server bridge
│   ├── logger/            # JSONL call logger
│   ├── vault/             # Encrypted vault (AES-256-GCM + HKDF)
│   └── oauth/             # OAuth 2.0 flow (PKCE + refresh)
├── test/                  # Integration tests
├── examples/              # Example configs
├── go.mod
└── go.sum
```

## Testing

Tests use `gotestsum` for formatted output:

- **Unit tests** — test each package in isolation. No external dependencies.
- **Integration tests** — build the binary and run end-to-end tests via subprocess. Build-tagged with `//go:build integration`.
- **MCP integration tests** — use Factorly itself as a child MCP server (no separate test binary).

---

[← Back to README](../README.md)
