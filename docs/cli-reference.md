# CLI Reference

## Commands

```bash
factorly serve                      # start MCP server (stdio)
factorly serve --http :3000         # start MCP server (HTTP at /mcp)
factorly serve --http-token <tok>   # HTTP with Bearer token auth
factorly serve --http-token '{{vault:HTTP_TOKEN}}'  # token from vault
factorly init                       # create .factorly/factorly.yaml (interactive)
factorly init --out factorly.yaml   # create at custom path
factorly tools                      # list all configured tools
factorly tools list                 # same as above
factorly tools add                  # add a tool (interactive)
factorly tools add --name x --type cli  # add a tool (non-interactive)
factorly tools remove <tool>        # remove a tool from config
factorly tools import openapi <spec>  # generate tools from OpenAPI spec
factorly tools import templates       # list available templates
factorly tools import templates <name>  # install a template (interactive)
factorly tools import templates <name> --dry-run  # preview YAML
factorly tools import templates <name> --all --api-key <key>  # non-interactive
factorly tools record                 # create a tool from a pasted curl command
factorly tools record --curl '...'    # create from inline curl
factorly tools record --dry-run       # preview YAML without writing
factorly call <tool> [--param val]  # call a tool
factorly sync                       # push MCP config to AI clients
factorly sync --global              # sync to user-level config (~/.claude/, etc.)
factorly sync --http localhost:3000 # sync HTTP mode
factorly sync --remove              # remove factorly from client configs
factorly tools status               # check all tools are reachable
factorly auth login <provider>      # OAuth login (opens browser)
factorly auth status [provider]     # show OAuth token status
factorly auth logout <provider>     # remove stored OAuth tokens
factorly vault set <key> [value]    # store a secret (prompts if no value)
factorly vault get <key>            # retrieve a secret (raw value to stdout)
factorly vault list                 # list secret names
factorly vault delete <key>         # remove a secret
factorly logs                       # view recent audit log entries
factorly logs -n 50                 # show last 50 entries
factorly logs --tool github         # filter by tool name
factorly logs --status blocked      # filter by status
factorly logs --detail              # show full entry details
factorly logs --stats               # show summary statistics
factorly logs -f                    # follow mode (tail -f style)
factorly exec -- <command> [args]   # run a command with compression + logging
factorly exec --compress json -- curl https://api.example.com  # with options
factorly wrap -- <command> [args]   # zero-config MCP proxy for any server
factorly wrap --url <url>          # proxy a remote MCP server
factorly version                    # print version
```

## Global flags

```bash
-v, --verbose          # print debug info to stderr
-c, --config <path>    # path to factorly.yaml
    --config-dir       # load tools from a directory (no config file needed)
```

## Vault flags

```bash
    --vault-path       # path to vault file (default: ~/.config/factorly/vault.enc)
```

## Environment variables

```bash
FACTORLY_VAULT_PASSWORD   # vault master password (for CI/automation)
FACTORLY_VAULT_PATH       # vault file path override
FACTORLY_HTTP_TOKEN       # HTTP server auth token (fallback for --http-token)
FACTORLY_NO_LOG           # disable call logging when set
FACTORLY_MAX_OUTPUT       # global max output bytes (fallback for per-tool max_output)
FACTORLY_DISABLED_TOOLS   # comma-separated tool names to disable (applies to call + tools listing)
```

## `factorly exec` — run a command with compression + logging

Runs a single shell command through Factorly's output processing and audit logging pipeline. The zero-config equivalent of a CLI tool definition.

```bash
factorly exec -- git status
factorly exec -- curl https://api.github.com/users/octocat
factorly exec --compress json -- npm test
factorly exec --env-isolation strict -- ./deploy.sh
```

### Flags

```bash
--max-output <bytes>      # max output bytes (default: 50000)
--compress <mode>         # compression: all, json, logs, none (default: "all")
--env-isolation <mode>    # "strict" for minimal env (default: inherit parent)
```

### What happens

1. Runs the command and captures stdout/stderr
2. Applies compression (ANSI strip, whitespace normalize, JSON compact, log dedup)
3. Truncates output if over `--max-output`
4. Logs to the JSONL audit trail (interface: `exec`)
5. Prints processed output and preserves the command's exit code

## `factorly wrap` — zero-config MCP proxy

Wraps any MCP server with Factorly's safety layer (compression, truncation, loop detection, rate limiting) — no config file needed. Tools keep their original names (no server prefix).

```bash
# Wrap a stdio MCP server
factorly wrap -- npx @modelcontextprotocol/server-github

# Wrap a remote HTTP MCP server
factorly wrap --url http://localhost:3001/mcp
```

### Flags

```bash
--url <url>          # connect to a remote MCP server instead of spawning a subprocess
--rate-limit <spec>  # rate limit (e.g. "100/hour")
--max-output <bytes> # max output bytes per call (default: 50000)
--compress <hints>   # compression hints: "json", "logs", or "all" (default: "all")
--http <addr>        # serve wrapped server over HTTP (e.g. ":3000")
--http-token <tok>   # Bearer token for HTTP mode
--env-isolation <mode>  # "strict" for minimal env, default inherits parent
```

### Defaults

- Compression: `all`
- Max output: `50000` bytes
- Loop detection: always-on

## HTTP server authentication

When running `factorly serve --http`, you can secure the endpoint with a Bearer token. The token is resolved in order:

1. `--http-token` flag
2. `FACTORLY_HTTP_TOKEN` environment variable

Both support `{{vault:KEY}}` references — the vault is opened automatically if a vault ref is detected:

```bash
# Plain token (visible in ps output — use for dev only)
factorly serve --http :3000 --http-token mytoken

# Environment variable (better for CI/deployment)
FACTORLY_HTTP_TOKEN=mytoken factorly serve --http :3000

# Vault reference (best — token encrypted at rest)
factorly vault set HTTP_TOKEN "my-secret-token"
factorly serve --http :3000 --http-token '{{vault:HTTP_TOKEN}}'

# Vault ref via env var
FACTORLY_HTTP_TOKEN='{{vault:HTTP_TOKEN}}' factorly serve --http :3000
```

When a token is set, all HTTP requests must include `Authorization: Bearer <token>`. Requests without a valid token receive a 401 response. A warning is printed to stderr when HTTP mode starts without any token configured.

**Note:** If Factorly is running inside a container (Docker, devcontainer, Codespace), use the host-accessible address — not `localhost`. For Docker, this is typically `host.docker.internal`:

```bash
factorly serve --http 0.0.0.0:3000 --http-token '{{vault:HTTP_TOKEN}}'
factorly sync --http host.docker.internal:3000 --token '{{vault:HTTP_TOKEN}}'
```

---

[← Back to README](../README.md)
