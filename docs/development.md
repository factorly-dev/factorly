# Development

Requires Go 1.24+.

## Setup

```bash
git clone https://github.com/factorly-dev/factorly.git
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
make version            # bump patch version, commit, tag (BUMP=minor|major)
make check-version      # verify Go + npm versions match
make test-coverage      # run tests with coverage report → build/coverage.out
make release            # cross-platform binaries (linux, darwin, windows)
```

## Project structure

```
src/
├── cmd/factorly/          # CLI entrypoint + commands
│   ├── main.go            # root command, call, init, tools
│   ├── serve_cmd.go       # factorly serve (MCP server)
│   ├── wrap_cmd.go        # factorly wrap (zero-config proxy)
│   ├── logs_cmd.go        # factorly logs (audit log viewer)
│   ├── templates_cmd.go   # factorly tools import templates
│   ├── health_cmd.go      # factorly tools status
│   ├── vault_cmd.go       # factorly vault set/get/list/delete
│   └── auth_cmd.go        # factorly auth login/status/logout
├── internal/
│   ├── config/            # YAML config loading + validation
│   ├── provider/          # CLI, REST, MCP providers + env isolation
│   ├── proxy/             # Proxy engine (route, log, output processing)
│   ├── registry/          # Tool registry
│   ├── server/            # MCP server bridge + agent identity
│   ├── logger/            # JSONL call logger
│   ├── vault/             # Encrypted vault (AES-256-GCM + HKDF)
│   ├── oauth/             # OAuth 2.0 flow (PKCE + refresh)
│   ├── shadow/            # Governance: deny, confirm, rate limit, loop detection
│   ├── output/            # Output compression + truncation
│   ├── agent/             # Agent identity registry
│   ├── naming/            # Name derivation utilities
│   ├── templates/         # Pre-built tool templates (36 services)
│   │   └── yaml/          # Embedded YAML tool definitions
│   ├── openapi/           # OpenAPI spec → tool config generator
│   └── parsing/curl/      # Curl command → tool config parser
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
- **Coverage** — `make test-unit` shows per-package coverage inline. `make test-coverage` generates `build/coverage.out` for `go tool cover -html`.

## Releasing

### 1. Run CI checks

```bash
make ci    # tidy, fmt, vet, lint, check-version, test
```

### 2. Bump version

```bash
make version              # 0.1.0 → 0.1.1 (patch)
make version BUMP=minor   # 0.1.0 → 0.2.0
make version BUMP=major   # 0.1.0 → 1.0.0
```

This updates both `src/internal/version.go` and `npm/package.json`, commits, and creates a git tag (`v0.1.1`).

### 3. Push with tags

```bash
git push && git push --tags
```

The GitHub Actions release workflow triggers on CI success for main:
- Builds cross-platform binaries (linux/amd64, darwin/amd64, darwin/arm64, windows/amd64)
- Creates a GitHub Release with the binaries attached
- Publishes the npm package (requires `NPM_TOKEN` secret in the `production` environment)

### Release checklist

1. `make ci` passes
2. `make version BUMP=patch|minor|major`
3. `git push && git push --tags`
4. Wait for GitHub Actions release workflow to complete
5. Verify: GitHub Releases page has binaries, `npx factorly version` returns new version

### Manual npm publish (if needed)

```bash
cd npm
npm publish
```

Requires `NPM_TOKEN` env var or `npm login`.

### Version files

| File | Updated by |
|------|-----------|
| `src/internal/version.go` | `make version` |
| `npm/package.json` | `make version` |

Both are committed and tagged together. `make check-version` (run by `make ci`) fails if they diverge.

---

[← Back to README](../README.md)
