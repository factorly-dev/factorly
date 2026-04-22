```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Release](https://img.shields.io/badge/Release-v0.7.0-blue?logo=github)](https://github.com/factorly-dev/factorly/releases)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-green.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen?logo=github)](https://github.com/factorly-dev/factorly/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/factorly)](https://www.npmjs.com/package/factorly)
[![PyPI](https://img.shields.io/pypi/v/factorly)](https://pypi.org/project/factorly/)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](docs/)

Stop giving your AI agents your API keys.

</center>

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault on your device. Every call is logged, governed, and rate-limited.

Install it, and your agent has safe access to GitHub, Slack, Stripe, and 30+ more services, plus any CLI or MCP server, in under a minute.

## Install

Install with your package manager of choice:

```bash
npm install -g factorly
  # or: pip install factorly
  # or: go install github.com/factorly-dev/factorly@latest
```

## Quick Start

Then, define your tools, secure your credentials, and sync with your agent:

```bash
# 1. Configure your tools or install a template (36 services: GitHub, Slack, Stripe, Linear, Gmail, ...)
factorly init

# 2. Store your credentials in the encrypted vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# 3. Connect to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync
```

The agent never sees your credentials. Factorly injected it, made the API call, logged it, and returned the data.

---

## The problem

Most MCP setups today expose secrets too broadly — API keys in `.env` files, OAuth tokens in config, credentials inherited from your user permissions. That means weak isolation, inconsistent policy enforcement, and incomplete audit trails.

## The solution

Your agent connects to Factorly as a single MCP server or CLI tool.

```
┌────────────┐       ┌────────────┐       ┌────────────┐
│            │       │            │       │            │
│ Your Agent │──────▶│  Factorly  │──────▶│ Your Tools │
│            │       │            │       │            │
└────────────┘       └────────────┘       └────────────┘
  Knows tools          Injects creds        GitHub, Slack,
  Knows params         Enforces policy      Stripe, REST,
  Makes requests       Logs everything      MCP, CLI
                       Rate-limits
  
  Never sees:
  API keys             ◀──────────────────  Returns only
  Tokens               data, never secrets  data
  Credentials
```

Factorly proxies every call, injecting real credentials server-side, enforcing policy, and logging everything.

The agent never handles secrets. The agent never bypasses governance.

---

## Features

MCP servers, REST APIs, CLI commands. One config, one endpoint, one audit log.

Your agent connects to Factorly once and sees all its approved tools.

### Encrypted vault

Your API keys, OAuth tokens, and secrets live in Factorly's fully encrypted local vault, using AES-256-GCM with per-entry encryption. Keys stay on your device. The agent sees tool names and data — never secrets.

```bash
# Store a secret — encrypted on disk, decrypted on demand
$ factorly vault set GITHUB_TOKEN
  Enter value: ••••••••••••••••

# Reference it in any tool config
  token: "{{vault:GITHUB_TOKEN}}"

# Your agent calls a tool — Factorly injects the secret
  Agent sees: data
  Agent never sees: ghp_xxxx...
```

### 36 templates

Pre-built configs for GitHub, Slack, Stripe, Linear, Gmail, Notion, Jira, HubSpot, Salesforce, and more. One command installs. One command connects to Claude Code, Cursor, or Codex.

```bash
$ factorly tools import templates github
$ factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx
$ factorly sync
```

### Wrap any MCP server

Already using an MCP server? Wrap it with zero config:

```bash
$ factorly wrap -- npx @modelcontextprotocol/server-github
```

Same tools, same interface. Now every call is logged, output is compressed, loops are detected, and calls are rate-limited.

### Governance

Block destructive operations. Require confirmation before writes. Rate-limit calls. Loop detection is always on — Factorly fingerprints identical calls and blocks runaway agents after 12 repeats.

```yaml
shadow:
  deny: [delete_repository, delete_branch]
  confirm: [merge_pull_request, create_release]
  rate_limit: 100/hour
```

Built-in tools block dangerous patterns like `rm -rf`, `curl | sh`, and `DROP TABLE` out of the box. Write and delete templates ship with `confirm: true` by default.

```bash
# Agent tries to run a destructive command
$ factorly call shell --command "rm -rf /"
  ✗ blocked: command matches deny pattern "rm -rf"

# Logged and denied. The command never executed.
```

### Audit trail

Every tool call logged: who called what, when, with what parameters, what was returned, what was blocked. Per-agent identity tracking for multi-agent setups.

```bash
$ factorly logs --tool github --status blocked
$ factorly logs -f    # follow in real time
```

### Output compression

Agent tools return too much data. Factorly compresses JSON, deduplicates log output, filters command-specific noise, and truncates to head + tail — saving tokens without losing signal. 27 built-in filters for common commands (git, make, npm, go, cargo, pytest, pip, docker, kubectl, terraform, and more) apply automatically. Savings tracked per-call in the audit log.

## Docs

- [Installation](docs/getting-started.md)
- [Configuration](docs/config-reference.md)
- [CLI Reference](docs/cli-reference.md)
- [Output Filters](docs/filters.md)
- [Workflows](docs/workflow-spec.md)
- [OAuth Setup](docs/oauth.md)
- [Audit Logging](docs/logging.md)
- [Template Library](docs/templates.md)
- [Examples](docs/examples/)

## License

[GPL-3.0](LICENSE)
