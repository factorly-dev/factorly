```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![PyPI](https://img.shields.io/pypi/v/factorly)](https://pypi.org/project/factorly/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-green.svg)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![GitHub](https://img.shields.io/badge/GitHub-factorly-181717?logo=github)](https://github.com/factorly-dev/factorly)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](https://github.com/factorly-dev/factorly/tree/main/docs)

Stop giving your AI agents your API keys.

</center>

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault on your device. Every call is logged, governed, and rate-limited.

Install it, and your agent has safe access to GitHub, Slack, Stripe, and 30+ more services, plus any CLI or MCP server, in under a minute.

## Install

```bash
pip install factorly
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

## Already Using an MCP Server?

Wrap it with Factorly — no config file needed, no changes to the server:

```bash
factorly wrap -- npx @modelcontextprotocol/server-github
```

Same tools, same interface. Now every call is logged, output is compressed, loops are detected, and calls are rate-limited.

## What You Get

- **Encrypted vault** — AES-256-GCM with per-entry encryption, secrets never leave your device
- **Governance** — deny destructive operations, require confirmation for writes, rate-limit calls, detect agent loops
- **Audit log** — every call logged with params, response, and governance outcome
- **36 templates** — pre-built configs for GitHub, Slack, Stripe, Gmail, Linear, and more
- **Zero-config proxy** — `factorly wrap` and `factorly exec` add safety instantly
- **Output compression** — compresses JSON, deduplicates logs, truncates to head + tail to save tokens

## Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | x64 |
| macOS | x64, arm64 |
| Windows | x64 |

The pip package downloads the pre-built Go binary for your platform on first run.

## Documentation

Full docs at [github.com/factorly-dev/factorly](https://github.com/factorly-dev/factorly)

## License

GPL-3.0
