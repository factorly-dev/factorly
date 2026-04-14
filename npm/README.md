```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/badge/Release-v0.2.0-blue?logo=github)](https://github.com/factorly-dev/factorly/releases)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)

One command. All your tools. Credentials stay out of your agent's hands.

</center>

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault. Every call is logged. Governance rules let you deny destructive operations, require approval for writes, rate-limit calls, and detect agent loops. REST APIs, CLI commands, and MCP servers — one config, one audit log, one set of rules.

## Install

```bash
npm install -g factorly
```

## Try It in 10 Seconds

```bash
# Wrap any MCP server with zero config
factorly wrap -- npx @modelcontextprotocol/server-everything

# Or install a template for a service you use
factorly tools import templates github
factorly call github.list_repos --username octocat
```

## Quick Start

```bash
factorly init                              # set up a project
factorly vault set GITHUB_TOKEN ghp_xxx    # store a secret
factorly sync                              # connect to Claude Code / Cursor
```

## What You Get

- **Credential isolation** — secrets in encrypted vault, agent never sees them
- **Governance** — deny, confirm, rate limit, loop detection per tool
- **Audit log** — every call logged with params, response, and governance outcome
- **36 templates** — pre-built configs for GitHub, Slack, Stripe, Gmail, Linear, and more
- **Zero-config proxy** — `factorly wrap` adds safety to any MCP server instantly

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

MIT
