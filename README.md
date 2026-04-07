```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/factorly-dev/factorly/ci.yml?label=CI&logo=github)](https://github.com/factorly-dev/factorly/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/factorly-dev/factorly?logo=github)](https://github.com/factorly-dev/factorly/releases)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)

Factorly wraps your existing agent tools — REST APIs, CLIs, MCP servers — into a single endpoint where credentials never reach the agent. Your agent sees tool names and parameters. Factorly injects the auth, makes the call, and returns the data.

</center>

```bash
# Your agent runs this — no secrets anywhere in the command
factorly call github.repos --username octocat --per_page 5

# Factorly injects the token, makes the HTTP call, returns the data
# The agent never sees GITHUB_TOKEN
```

## Why

Your AI agent needs to call Slack, GitHub, Stripe, a database, an internal API. Today that means:

- **Secrets in the agent's context** — every API key is one prompt injection away from exposure
- **Auth logic duplicated** across every tool, every project
- **No audit trail** — what did the agent actually call? With what parameters?
- **Key rotation** means hunting through agent configs and .env files

Factorly fixes this. Secrets live in Factorly's config — or encrypted in the vault. The agent only knows tool names.

```
┌──────────────────────┐         ┌──────────────────────────────┐
│  Your Agent          │         │  Factorly                    │
│                      │         │                              │
│  Knows:              │  call   │  Injects:                    │
│  - tool names        │────────▶│  - Authorization headers     │
│  - parameter names   │         │  - API keys from vault       │
│                      │◀────────│  - Base URLs                 │
│  Never sees:         │  data   │                              │
│  - API keys          │         │  Logs every call.            │
│  - tokens            │         │  Returns only data.          │
│  - credentials       │         │                              │
└──────────────────────┘         └──────────────────────────────┘
```

## Quick Start

```bash
# Install
git clone https://github.com/factorly-dev/factorly.git
cd factorly && make init && make build

# Initialize a project
factorly init

# Add a tool
factorly tools add --name web.fetch --type cli --command curl --args '-s,{url}'

# Use it on the command line
factorly call web.fetch --url "https://example.com"

# Connect to Claude Code / Cursor / Codex
factorly sync

# Or start manually as an MCP server
factorly serve
```

### Configure your tools

```yaml
# .factorly/factorly.yaml
tools:
  web.fetch:
    type: cli
    description: "Fetch a webpage"
    command: curl
    args: ["-s", "{url}"]

  github.repos:
    type: rest
    description: "List repos for a user"
    base_url: https://api.github.com
    method: GET
    path: /users/{username}/repos
    auth:
      type: bearer
      token: "${vault:GITHUB_TOKEN}"
    parameters:
      - name: username
        in: path
        required: true

  slack:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-slack"]
    env:
      SLACK_TOKEN: ${vault:SLACK_TOKEN}
```

See the full [Config Reference](docs/config-reference.md) for all options.

## Connect to Your Agent

### Claude Code

```json
// .mcp.json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

### Cursor

```json
// .cursor/mcp.json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

### OpenAI Codex

```json
// .codex/mcp.json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

For HTTP mode (remote/shared): `factorly serve --http :3000` — endpoint at `http://localhost:3000/mcp`. Secure with `--http-token` or `FACTORLY_HTTP_TOKEN`. See [CLI Reference](docs/cli-reference.md) for all options.

> **Note on MCP authorization:** The [MCP spec](https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization) defines OAuth 2.1 as the standard HTTP auth mechanism, but notes it is **optional**. Factorly currently uses static Bearer token auth (`--http-token`), which works with all major MCP clients (Claude Code, Cursor, Codex). OAuth 2.1 server support is [planned](docs/oauth-server-spec.md) for multi-tenant and hosted deployments. For production OAuth today, place Factorly behind a reverse proxy (nginx, Caddy) with OAuth at the proxy layer.

## What You Get

- **One endpoint** — your agent connects to Factorly, sees everything
- **Credentials secured** — secrets live in the encrypted vault or env vars, never in the agent
- **Every call logged** — every tool call is logged with timestamp, parameters, and response summary
- **Zero lock-in** — your tools don't change. Remove Factorly and everything still works independently
- **Any protocol** — MCP servers, REST APIs, CLI tools. One config format for all of them

| Type | How It Works | Status |
|---|---|---|
| **CLI commands** | Define command + args in YAML. `{param}` placeholders substituted. | Working |
| **REST APIs** | Define base URL, method, path, auth, parameters. HTTP calls with routing. | Working |
| **MCP servers** | Spawn child servers (stdio) or connect to remote (HTTP). Tools discovered automatically. | Working |

## Documentation

| Topic | Description |
|-------|-------------|
| [Config Reference](docs/config-reference.md) | Full YAML schema, auth types, parameters, stdin, interactive mode |
| [CLI Reference](docs/cli-reference.md) | All commands, subcommands, and flags |
| [Vault](docs/vault.md) | Encrypted secret storage with per-entry encryption |
| [OAuth](docs/oauth.md) | OAuth 2.0 authentication with PKCE and auto-refresh |
| [OpenAPI Import](docs/openapi-import.md) | Generate tools from OpenAPI/Swagger specs |
| [Project Directory](docs/project-directory.md) | Modular configs with `.factorly/` and `tools_dir` |
| [Logging](docs/logging.md) | Call log format, location, and security |
| [OAuth Server Spec](docs/oauth-server-spec.md) | Planned: MCP-spec OAuth 2.1 authorization server design |
| [Development](docs/development.md) | Building, testing, and contributing |

## Examples

- **[GitHub OAuth](src/examples/github-oauth.yaml)** — REST tools with OAuth authentication
- **[Basic CLI tools](src/examples/factorly.yaml)** — Simple CLI tool wrapping
- **[Secure Agent](src/examples/secure-agent/)** — Full example with modular tool files

## Roadmap

- [x] CLI provider — wrap shell commands as tools
- [x] REST provider — wrap HTTP APIs as tools
- [x] MCP provider — spawn child servers (stdio) or connect to remote (HTTP)
- [x] `factorly serve` — MCP server mode (stdio + HTTP)
- [x] `factorly tools` — list, add, remove, import
- [x] `factorly status` — health check all tools and connections
- [x] `factorly init` — interactive project setup
- [x] Encrypted vault — `${vault:KEY}` with per-entry encryption (HKDF + AES-256-GCM)
- [x] OAuth authentication — `factorly auth login/status/logout` with PKCE + auto-refresh
- [x] Interactive CLI tools — `interactive: true` for TTY passthrough
- [x] Call logging (JSONL) + `--verbose` flag
- [x] Security hardening — param redaction, HTTP token auth, vault ref validation
- [x] `factorly sync` — push MCP config into AI clients (Claude Code, Cursor, Codex)
- [ ] OAuth 2.1 server — [MCP-spec auth](docs/oauth-server-spec.md) with PKCE, dynamic registration, token endpoints
- [ ] `factorly logs` — view/query the call log
- [ ] External vault backends (1Password, GCP Secret Manager, AWS)
- [ ] Hosted version (Factorly Cloud)
- [ ] Team configs and shared credential vault
- [ ] Dashboard and audit log UI

## Philosophy

- **Wrap, don't replace.** Your tools don't change. Factorly sits in front.
- **Zero lock-in.** Remove Factorly and everything still works.
- **Credentials are not your agent's business.** Secrets live in Factorly, not in your agent.
- **Log everything.** If an agent did it, there's a record.

## License

MIT
