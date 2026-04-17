```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![npm](https://img.shields.io/npm/v/factorly)](https://www.npmjs.com/package/factorly)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-green.svg)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![GitHub](https://img.shields.io/badge/GitHub-factorly-181717?logo=github)](https://github.com/factorly-dev/factorly)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](https://github.com/factorly-dev/factorly/tree/main/docs)

One command. All your tools. Credentials stay out of your agent's hands.

</center>

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault. Every call is logged. Governance rules let you deny destructive operations, require approval for writes, rate-limit calls, and detect agent loops. REST APIs, CLI commands, and MCP servers — one config, one audit log, one set of rules.

## Install

```bash
npm install -g factorly
```

## Quick Start

```bash
# 1. Install a template (36 services: GitHub, Slack, Stripe, Linear, Gmail, ...)
factorly tools import templates github

# 2. Store your credentials in the encrypted vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# 3. Connect to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync
```

The agent never sees your GitHub token. Factorly injected it, made the API call, logged it, and returned the data.

## Already Using an MCP Server?

Wrap it with Factorly — no config file needed, no changes to the server:

```bash
factorly wrap -- npx @modelcontextprotocol/server-github
```

## What You Get

- **Credential isolation** — secrets in encrypted vault, agent never sees them
- **Governance** — deny, confirm, rate limit, loop detection per tool
- **Audit log** — every call logged with params, response, and governance outcome
- **36 templates** — pre-built configs for GitHub, Slack, Stripe, Gmail, Linear, and more
- **Zero-config proxy** — `factorly wrap` and `factorly exec` add safety instantly

## Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | x64 |
| macOS | x64, arm64 |
| Windows | x64 |

The npm package downloads the pre-built Go binary for your platform during install.

## Documentation

Full docs at [github.com/factorly-dev/factorly](https://github.com/factorly-dev/factorly)

## License

GPL-3.0
