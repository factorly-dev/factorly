```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen?logo=github)](https://github.com/factorly-dev/factorly-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/Release-v0.1.7-blue?logo=github)](https://github.com/factorly-dev/factorly-cli/releases)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![npm](https://img.shields.io/npm/v/@factorly/cli)](https://www.npmjs.com/package/@factorly/cli)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](docs/)

One command. All your tools. Credentials stay out of your agent's hands. 

</center>

Factorly is a security and governance layer between AI agents and the tools they use. REST APIs, CLI commands, and MCP servers — one config, one audit log, one set of rules.

Your agent sees tool names and data. Never secrets.

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

## Install

```bash
npm install -g @factorly/cli
```

Or with Go:

```bash
go install github.com/factorly-dev/factorly-cli@latest
```

Or build from source:

```bash
git clone https://github.com/factorly-dev/factorly-cli.git
cd factorly-cli && make build
# binary at build/factorly
```

## Try It in 10 Seconds

Wrap any existing MCP server — instant audit logging, output compression, loop detection, and rate limiting with zero config:

```bash
factorly wrap -- npx @modelcontextprotocol/server-everything
```

Or install a pre-built template for a service you already use:

```bash
factorly tools import templates github
factorly call github.list_repos --username octocat
```

36 templates available: GitHub, Slack, Linear, Stripe, Notion, Gmail, Telegram, and more. Run `factorly tools import templates` to see the full list.

## Quick Start

```bash
# 1. Initialize a project
factorly init

# 2. Store a secret
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# 3. Connect to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync

# That's it. Your agent now has access to your tools via MCP.
```

### What your agent sees

Once synced, your agent discovers Factorly's tools automatically. In Claude Code:

```
> List my GitHub repos

I'll use the github.repos tool to look that up.

[Calling github.repos with username=octocat]

Found 30 repositories. Here are the first 5:
1. octocat/Hello-World
2. octocat/Spoon-Knife
...
```

The agent never sees your GitHub token. Factorly injected it, made the API call, logged it, and returned the data.

## Configure Tools

The simplest tool is three lines:

```yaml
# .factorly/factorly.yaml
tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
```

A REST API with auth:

```yaml
  github.repos:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /users/{{username}}/repos
    auth:
      type: bearer
      token: "{{vault:GITHUB_TOKEN}}"
```

An existing MCP server:

```yaml
  slack:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-slack"]
    env:
      SLACK_TOKEN: "{{vault:SLACK_TOKEN}}"
```

Or skip YAML entirely — use a [template](docs/templates.md) or [import from an OpenAPI spec](docs/openapi-import.md).

## Secrets Never Leave Factorly

Secrets live in an encrypted vault (AES-256-GCM, Argon2id key derivation, per-entry encryption). Your agent config has zero credentials in it.

```bash
# Store secrets
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx
factorly vault set STRIPE_KEY sk_live_xxxxxxxxxxxx

# Reference them in your tool config
# token: "{{vault:GITHUB_TOKEN}}"

# List what's stored
factorly vault list
```

The vault password can come from an environment variable (`FACTORLY_VAULT_PASSWORD`), a key file (`~/.config/factorly/vault.key`), or an interactive prompt. See [Vault docs](docs/vault.md).

## What You Get

| Feature | Description |
|---------|-------------|
| **Credential isolation** | Secrets in encrypted vault (AES-256-GCM). Agent never sees them. |
| **Governance** | Deny, confirm, rate limit, and loop detection per tool. |
| **Audit log** | Every call logged — who, what, when, with what params, what was returned. |
| **Output efficiency** | Compression + truncation saves agent context window. |
| **36 templates** | Pre-built configs for GitHub, Slack, Stripe, Gmail, Linear, Telegram, and more. |
| **Zero-config proxy** | `factorly wrap` adds safety to any MCP server instantly. |
| **Any protocol** | REST APIs, CLI commands, MCP servers. One config format. |
| **OAuth 2.0** | PKCE flow with auto-refresh for Google, GitHub, Microsoft, Slack. |

## Commands

```bash
factorly init                              # interactive project setup
factorly tools                             # list configured tools
factorly tools import templates            # browse 36 pre-built templates
factorly tools import templates linear     # install Linear tools (30 sec)
factorly call <tool> [--param val]         # call a tool
factorly serve                             # start MCP server (stdio)
factorly wrap -- <command> [args]          # zero-config MCP proxy
factorly sync                              # push config to AI clients
factorly status                            # health check all tools
factorly logs                              # view audit log
factorly vault set <key> [value]           # store a secret
factorly auth login <provider>             # OAuth login
```

Full reference: [CLI Reference](docs/cli-reference.md)

## Documentation

| Topic | |
|-------|---|
| [Getting Started](docs/getting-started.md) | Install, configure, connect |
| [Config Reference](docs/config-reference.md) | Full YAML schema |
| [CLI Reference](docs/cli-reference.md) | All commands and flags |
| [Templates](docs/templates.md) | 36 pre-built service configs |
| [Vault](docs/vault.md) | Encrypted secret storage |
| [OAuth](docs/oauth.md) | OAuth 2.0 with PKCE |
| [Logging](docs/logging.md) | Audit log format and querying |

## License

MIT
