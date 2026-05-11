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

**Build what your agent can do.**

Define tools, compose workflows, test and run them.
MCP servers, REST APIs, and CLI commands in one config, one UI, one audit log.

</center>

Factorly is a local runtime for agent tool chains. It manages tool calls, injects credentials from an encrypted vault, enforces governance rules, and logs everything. Your agent sees workflows, tools, and data. Secrets stay secret.

## Install

```bash
npm install -g factorly
```

## Quick Start

```bash
# 1. Configure your tools or install a template (36 services: GitHub, Slack, Stripe, Linear, Gmail, ...)
factorly init

# 2. Store your credentials in the encrypted vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# 3. Connect to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync

# 4. Optional, start the UI
factorly ui
```

Your agent connects to Factorly as a single MCP server or CLI and sees every tool you've configured. Credentials never leave the vault.

## What It Does

**Define** — one config, every protocol, 36 templates included

**Test** — Try tools in the UI, see the response, iterate before giving your agent access

**Compose** — workflows with per-step policies, deterministic sequences

**Govern** — vault, policies, audit log. Built in, not bolted on.

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
