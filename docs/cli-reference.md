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
factorly logs -f                    # follow mode (tail -f style)
factorly logs stats                 # show summary statistics
factorly logs verify                # verify hash chain integrity
factorly logs repair                # repair broken hash chain
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
    --vault-path       # explicit path to vault file (overrides auto-detection)
    --global           # use global vault (~/.config/factorly/vault.enc) instead of project vault
```

## Environment variables

```bash
FACTORLY_VAULT_PASSWORD           # global vault password (also shared fallback for project vault)
FACTORLY_PROJECT_VAULT_PASSWORD   # project vault password (.factorly/vault.enc)
FACTORLY_VAULT_PATH               # vault file path override
FACTORLY_HTTP_TOKEN       # HTTP server auth token (fallback for --http-token)
FACTORLY_NO_LOG           # disable call logging when set
FACTORLY_MAX_OUTPUT       # global max output bytes (fallback for per-tool max_output)
FACTORLY_DISABLED_TOOLS   # comma-separated tool names to disable (applies to call + tools listing)
```

## `factorly exec` — run a command with compression + logging

Runs a single shell command through Factorly's full safety layer — the zero-config equivalent of a CLI tool definition. Uses the same code path as config-based CLI tools.

```bash
factorly exec -- git status
factorly exec --compress json -- npm test
factorly exec --env-isolation strict -- ./deploy.sh
factorly exec -i -- psql -h localhost mydb
```

Supports `{{vault:KEY}}` and `{{env:VAR}}` references in arguments — secrets stay out of shell history:

```bash
factorly exec -- curl -H "Authorization: Bearer {{vault:GITHUB_TOKEN}}" https://api.github.com/user
```

### Flags

```bash
--max-output <bytes>      # max output bytes (default: 50000)
--compress <mode>         # compression: all, json, logs, none (default: "all")
--env-isolation <mode>    # "strict" for minimal env (default: inherit parent)
--env KEY=VALUE           # set env var (repeatable; supports {{env:VAR}} and {{vault:KEY}})
-i, --interactive         # connect directly to terminal (skip compression, for TTY tools)
--timeout <duration>      # execution timeout (e.g. 30s, 5m; default: 30s)
```

Use `--env` with `--env-isolation strict` to pass specific vars into a restricted environment:

```bash
factorly exec --env-isolation strict --env DISPLAY={{env:DISPLAY}} -- xsel --clipboard
factorly exec --env-isolation strict --env GITHUB_TOKEN={{vault:GITHUB_TOKEN}} -- gh pr list
```

### What happens

1. Resolves `{{env:VAR}}` and `{{vault:KEY}}` references in arguments
2. Runs the command through the CLI provider (same as config-based tools)
3. Applies built-in output filters for recognized commands (git, make, npm, go test, cargo, pip)
4. Applies compression and truncation (unless `-i` interactive mode)
5. Logs to the JSONL audit trail (interface: `exec`)
6. Prints processed output and preserves the command's exit code

Built-in filters automatically reduce noise for common commands — for example, `factorly exec -- go test ./...` will short-circuit to `"ok (all tests passed)"` on success, or show only PASS/FAIL lines on failure. See [Output Filters](filters.md) for the full list and how to define custom filters.

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
--env KEY=VALUE         # set env var (repeatable; supports {{env:VAR}} and {{vault:KEY}})
--timeout <duration>    # tool call timeout (e.g. 30s, 5m; default: 30s)
```

### Defaults

- Compression: `all`
- Max output: `50000` bytes
- Loop detection: always-on

## Built-in Tools

Factorly ships with governed alternatives to common agent tools. These are available automatically — no YAML config needed. All prefixed `factorly.` to avoid collision with user-defined tools.

| Tool | Description | Default governance |
|------|-------------|-------------------|
| `factorly.shell` | Run a shell command | Confirm required, destructive patterns blocked |
| `factorly.read_file` | Read a local file | Sensitive paths blocked (.env, .ssh, credentials) |
| `factorly.write_file` | Write a local file | Confirm required, system paths blocked |
| `factorly.fetch` | HTTP GET a URL | Cloud metadata + private networks blocked |
| `factorly.clipboard` | Copy text to clipboard | Confirm required |

### Context-aware

In **stdio mode** (default): all 5 tools available.
In **HTTP mode** (`--http`): only `factorly.fetch` — local tools don't make sense on a remote server.

### Safety guards

Built-in tools have default safety restrictions that block dangerous operations before execution:

**Shell** — blocks `rm -rf /`, `curl | sh`, `DROP TABLE`, `shutdown`, fork bombs, and similar destructive patterns.

**Read/Write** — blocks `.env`, `.ssh/id_*`, `*.pem`, `credentials.json`, `/etc/shadow`, system directories (`/etc/`, `/usr/`, `/bin/`), and shell configs.

**Fetch** — blocks cloud metadata (`169.254.169.254`), localhost, private networks (`10.*`, `172.16-31.*`, `192.168.*`), and `file://` protocol.

### Allow overrides

Override default denials for specific cases:

```yaml
tools:
  factorly.shell:
    shadow:
      allow_patterns: ["rm -rf ./build"]  # permit this specific command

  factorly.read_file:
    shadow:
      allow_paths: [".env.example"]  # permit reading this file

  factorly.fetch:
    shadow:
      allow_urls: ["http://localhost:8080"]  # permit local dev server
```

### Disabling

```bash
# Disable specific built-ins
FACTORLY_DISABLED_TOOLS=factorly.shell,factorly.write_file
```

Or disable all built-ins in config:

```yaml
disable_builtins: true
```

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
