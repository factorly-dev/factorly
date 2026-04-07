```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen?logo=github)](https://github.com/factorly-hq/factorly-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/Release-v0.1.0-blue?logo=github)](https://github.com/factorly-hq/factorly-cli/releases)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](docs/)

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
git clone https://github.com/factorly-hq/factorly-cli.git
cd factorly && make init && make build

# Initialize a project
factorly init

# Add a tool from a curl command
echo 'curl -H "Authorization: Bearer $TOKEN" https://api.example.com/data' | factorly tools record

# Or add interactively
factorly tools add --name web.fetch --type cli --command curl --args '-s,{url}'

# Use it
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

## Documentation

Full documentation is in the **[docs/](docs/)** directory — [getting started](docs/getting-started.md), config reference, CLI reference, vault, OAuth, OpenAPI import, and more.

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
- [x] `factorly sync` — push MCP config into AI clients (Claude Code, Cursor, Codex)
- [x] Encrypted vault — `${vault:KEY}` with per-entry encryption (HKDF + AES-256-GCM)
- [x] OAuth authentication — `factorly auth login/status/logout` with PKCE + auto-refresh
- [x] Interactive CLI tools — `interactive: true` for TTY passthrough
- [x] Call logging (JSONL) + `--verbose` flag
- [x] Security hardening — param redaction, HTTP token auth, vault ref validation
- [ ] OAuth 2.1 server — [MCP-spec auth](docs/oauth-server-spec.md) with PKCE, dynamic registration, token endpoints
- [ ] `factorly logs` — view/query the call log
- [ ] External vault backends (1Password, GCP Secret Manager, AWS)
- [ ] Hosted version (Factorly Cloud)
- [ ] Team configs and shared credential vault
- [ ] Dashboard and audit log UI

## License

MIT
