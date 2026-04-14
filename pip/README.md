```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

# Factorly

One command. All your tools. Credentials stay out of your agent's hands.

Factorly is a local proxy between your AI agent and the tools it uses. Secrets stay in an encrypted vault. Every call is logged. Governance rules let you deny destructive operations, require approval for writes, rate-limit calls, and detect agent loops. REST APIs, CLI commands, and MCP servers — one config, one audit log, one set of rules.

## Install

```bash
pip install factorly
```

Or with pipx:

```bash
pipx install factorly
```

## Try It in 10 Seconds

```bash
# Run any command with compression + logging
factorly exec -- git status

# Wrap any MCP server with zero config
factorly wrap -- npx @modelcontextprotocol/server-everything

# Install a template for a service you use
factorly tools import templates github
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
- **Zero-config proxy** — `factorly wrap` and `factorly exec` add safety instantly

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

MIT
