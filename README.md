```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Release](https://img.shields.io/badge/Release-v0.5.4-blue?logo=github)](https://github.com/factorly-dev/factorly/releases)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-green.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen?logo=github)](https://github.com/factorly-dev/factorly/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/factorly)](https://www.npmjs.com/package/factorly)
[![PyPI](https://img.shields.io/pypi/v/factorly)](https://pypi.org/project/factorly/)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](docs/)

One command. All your tools. Credentials stay out of your agent's hands.

</center>

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault. Every call is logged. Governance rules let you deny destructive operations, require approval for writes, rate-limit calls, and detect agent loops. REST APIs, CLI commands, and MCP servers — one config, one audit log, one set of rules.

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

**npm:** `npm install -g factorly`

**pip:** `pip install factorly`

**go:** `go install github.com/factorly-dev/factorly@latest`

## Quick Start

```bash
# 1. Install a template (36 services: GitHub, Slack, Stripe, Linear, Gmail, ...)
factorly tools import templates github

# 2. Store your credentials in the encrypted vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# 3. Connect to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync
```

That's it. Your agent now discovers Factorly's tools via MCP:

```
> List my GitHub repos

I'll use the github.list_repos tool to look that up.

[Calling github.list_repos with username=octocat]

Found 30 repositories:
1. octocat/Hello-World
2. octocat/Spoon-Knife
...
```

The agent never sees your GitHub token. Factorly injected it, made the API call, logged it, and returned the data.

## Already Using an MCP Server?

Wrap it with Factorly — no config file needed, no changes to the server:

```bash
factorly wrap -- npx @modelcontextprotocol/server-github
```

Your agent connects to Factorly instead of the MCP server directly. Same tools, same interface — but now every call is logged, output is compressed, loops are detected, and calls are rate-limited.

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Install, configure, connect to your agent |
| [Config Reference](docs/config-reference.md) | Full YAML schema for CLI, REST, and MCP tools |
| [CLI Reference](docs/cli-reference.md) | All commands, flags, and environment variables |
| [Templates](docs/templates.md) | 36 pre-built configs for popular services |
| [Vault](docs/vault.md) | Encrypted secret storage (AES-256-GCM) |
| [OAuth](docs/oauth.md) | OAuth 2.0 with PKCE and auto-refresh |
| [Logging](docs/logging.md) | Audit log format, querying, and `factorly logs` |
| [Examples](docs/examples/) | 27 practical, copy-paste examples for every feature |

## License

GPL-3.0
