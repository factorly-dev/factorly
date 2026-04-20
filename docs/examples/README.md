# Examples

Practical, copy-paste examples for every Factorly feature. Each is self-contained and takes under 2 minutes to read.

## Getting Started

| # | Example | Description |
|---|---------|-------------|
| 1 | [Hello World](01-hello-world.md) | Simplest possible tool — wrap `echo` as a callable CLI tool |
| 2 | [REST API with Bearer Token](02-rest-api-with-bearer-token.md) | Call the GitHub API with a vault-stored token |
| 3 | [Install a Template](03-install-a-template.md) | Get a fully configured Slack integration in 30 seconds |
| 4 | [Wrap an MCP Server](04-wrap-an-mcp-server.md) | Proxy any MCP server through Factorly with zero config |
| 5 | [Connect to Claude Code](05-connect-to-claude-code.md) | Full onboarding: init, add tools, store credentials, sync |

## Authentication

| # | Example | Description |
|---|---------|-------------|
| 6 | [OAuth with GitHub](06-oauth-with-github.md) | Browser-based OAuth 2.0 with automatic token refresh |
| 7 | [Custom Header Authentication](07-custom-header-auth.md) | Use a custom `X-API-Key` header instead of Bearer |
| 8 | [Basic Authentication](08-basic-auth.md) | HTTP Basic Auth for legacy APIs and self-hosted services |
| 9 | [External Secrets with 1Password](09-external-secrets-1password.md) | Pull secrets from 1Password CLI instead of the built-in vault |

## Governance

| # | Example | Description |
|---|---------|-------------|
| 10 | [Deny Dangerous Operations](10-deny-dangerous-operations.md) | Block destructive tool calls before they reach the service |
| 11 | [Require Confirmation](11-require-confirmation.md) | Require human approval before high-impact operations |
| 12 | [Rate Limit API Calls](12-rate-limit-api-calls.md) | Prevent runaway agents from burning through API quotas |
| 13 | [Lock Down for Production](13-lock-down-for-production.md) | Hardened config: deny destructive ops, rate limit, confirm writes |

## Integration

| # | Example | Description |
|---|---------|-------------|
| 14 | [GraphQL API Integration](14-graphql-api.md) | Integrate a GraphQL API (Linear) using the REST tool type |
| 15 | [Custom Internal API](15-custom-internal-api.md) | Connect to a company-internal REST API with path and body params |
| 16 | [Wrap a CLI Tool](16-wrap-a-cli-tool.md) | Expose a team script as a governed tool with vault secrets |
| 17 | [Telegram Bot Notifications](17-telegram-bot-notifications.md) | Send messages to Telegram from your agent |
| 18 | [Interactive Database Shell](18-database-shell.md) | Launch a live database session with vault-injected credentials |

## Project Structure

| # | Example | Description |
|---|---------|-------------|
| 19 | [Multi-File Project Structure](19-multi-file-project.md) | Split tools across multiple YAML files in `.factorly/tools/` |
| 20 | [Per-Project Vault](20-per-project-vault.md) | Separate project secrets from global personal secrets |

## Safety and Environment

| # | Example | Description |
|---|---------|-------------|
| 21 | [Environment Isolation](21-environment-isolation.md) | Restrict child process environment to essential variables only |
| 22 | [Output Compression](22-output-compression.md) | Compact JSON, collapse repeated log lines, strip ANSI codes |
| 23 | [Large Output Truncation](23-large-output-truncation.md) | Cap output size with head+tail truncation |
| 28 | [Output Filters](28-output-filters.md) | Strip noise, short-circuit success, keep/replace lines, head/tail |
| 29 | [Pipe Filters](29-pipe-filters.md) | Pipe output through jq, grep, sed, or custom scripts |

## CLI Power Tools

| # | Example | Description |
|---|---------|-------------|
| 24 | [Ad-hoc Commands with exec](24-exec-for-ad-hoc-commands.md) | Run any command through Factorly's safety layer without config |
| 25 | [MCP over HTTP](25-mcp-http-remote.md) | Serve tools over HTTP for remote agents and containers |
| 26 | [Built-in Tools](26-built-in-tools.md) | Governed shell, file, fetch, and clipboard tools out of the box |

## Compliance

| # | Example | Description |
|---|---------|-------------|
| 27 | [Audit and Compliance](27-audit-and-compliance.md) | Log params for compliance, query audit trails, track blocked calls |

---

[← Back to Documentation](../README.md)
