# Factorly Documentation

## What You Get

- **One endpoint** — your agent connects to Factorly, sees everything
- **Credentials secured** — secrets live in the encrypted vault or env vars, never in the agent
- **Every call logged** — every tool call is logged with timestamp, parameters, and response summary
- **Zero lock-in** — your tools don't change. Remove Factorly and everything still works independently
- **Any protocol** — MCP servers, REST APIs, CLI tools. One config format for all of them

| Type | How It Works | Status |
|---|---|---|
| **CLI commands** | Define command + args in YAML. `{{param}}` placeholders substituted. | Working |
| **REST APIs** | Define base URL, method, path, auth, parameters. HTTP calls with routing. | Working |
| **MCP servers** | Spawn child servers (stdio) or connect to remote (HTTP). Tools discovered automatically. | Working |

## Getting Started

| Topic | Description |
|-------|-------------|
| [Getting Started](getting-started.md) | Install, configure, connect to your agent |

## Reference

| Topic | Description |
|-------|-------------|
| [Config Reference](config-reference.md) | Full YAML schema, auth types, parameters, stdin, interactive mode |
| [CLI Reference](cli-reference.md) | All commands, subcommands, flags, and environment variables |

## Features

| Topic | Description |
|-------|-------------|
| [Vault](vault.md) | Encrypted secret storage with per-entry encryption |
| [OAuth](oauth.md) | OAuth 2.0 authentication with PKCE and auto-refresh |
| [OpenAPI Import](openapi-import.md) | Generate tools from OpenAPI/Swagger specs |
| [Project Directory](project-directory.md) | Modular configs with `.factorly/` and `tools_dir` |
| [Logging](logging.md) | Call log format, location, and security |

## Contributing

| Topic | Description |
|-------|-------------|
| [Development](development.md) | Building, testing, and project structure |

## Specs

| Topic | Description |
|-------|-------------|
| [OAuth Server Spec](oauth-server-spec.md) | Planned: MCP-spec OAuth 2.1 authorization server design |

## Philosophy

- **Wrap, don't replace.** Your tools don't change. Factorly sits in front.
- **Zero lock-in.** Remove Factorly and everything still works.
- **Credentials are not your agent's business.** Secrets live in Factorly, not in your agent.
- **Log everything.** If an agent did it, there's a record.

---

[← Back to README](../README.md)
