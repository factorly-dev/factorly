# CLI Reference

## Commands

```bash
factorly serve                      # start MCP server (stdio)
factorly serve --port 3000          # start MCP server (HTTP at /mcp)
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
factorly blueprint list               # list installed blueprints
factorly blueprint install <name>     # install a bundled blueprint (e.g. "linear", "gmail")
factorly blueprint install <src>      # install from URL, github.com/owner/repo, or local file
factorly blueprint install <src> --dry-run   # preview without writing
factorly blueprint install <src> --no-prompt # skip interactive vault-key prompts
factorly blueprint uninstall <name>   # remove an installed blueprint
factorly tools record                 # create a tool from a pasted curl command
factorly tools record --curl '...'    # create from inline curl
factorly tools record --dry-run       # preview YAML without writing
factorly call <tool> [--param val]  # call a tool (cli, rest, mcp, or workflow)
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
factorly version                    # print version (uses 24h cache)
factorly version --check            # force version check (bypass cache)
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

## `factorly call` — parameter passing

Parameters are passed as `--name value` pairs:

```bash
factorly call api.users --limit 10 --status active
```

### Read from stdin

Use `-` to read a parameter value from stdin. Only one parameter per call can use stdin.

```bash
# Pipe input
echo "What is the capital of France?" | factorly call anthropic.ask --prompt -

# Heredoc
factorly call anthropic.ask --prompt - <<'EOF'
Explain this code:
func main() { fmt.Println("hello") }
EOF
```

### Read from file

Use `@filename` to read a parameter value from a file (like curl's `--data @file.json`):

```bash
factorly call anthropic.ask --prompt @prompt.txt
factorly call api.upload --body @data.json
```

To pass a literal `@` prefix, escape with `@@`:

```bash
factorly call tool --email @@user    # sends @user
```

## Workflows

Workflows are tools with `type: workflow` — call them like any other tool:

```bash
factorly call deploy.staging --branch main
factorly call deploy.staging --branch main -v    # verbose: see each step
```

### Verbose output

With `-v`, each step streams progress to stderr:

```
[workflow] deploy.staging started (run: a1b2c3d4)
[workflow]   1/4 git.branch              running...
[workflow]   1/4 git.branch              completed  12ms
[workflow]   2/4 make.test               running...
[workflow]   2/4 make.test               completed  5.2s
[workflow]   3/4 make.deploy             skipped    (condition false)
[workflow]   4/4 slack.notify            running...
[workflow]   4/4 slack.notify            completed  342ms
[workflow] deploy.staging completed (4 steps, 5.6s)
```

### Conditional steps

Steps can have `if:` (skip when false), `require:` (stop workflow when false), and `switch:` (multi-branch):

```yaml
steps:
  - tool: make.test
    store: output
    if: "branch == 'main'"        # skip this step if not main

  - tool: make.deploy
    require: "contains(output, 'PASS')"  # stop workflow if tests failed

  - switch:                        # multi-branch
      - condition: "contains(output, 'PASS')"
        tool: slack.post
        params: { text: "✓ passed" }
      - condition: "true"          # default
        tool: slack.post
        params: { text: "✗ failed" }
```

- `if:` — skip one step, continue
- `require:` — stop entire workflow (graceful, status = completed)
- `switch:` — first matching condition executes

Conditions use the [expression language](expressions.md): comparisons (`==`, `!=`, `>`, `<`), boolean logic (`and`, `or`, `not`), `contains()`, `jsonpath()`, and member access (`response.code`).

### Output format

Workflow output is JSON with the full execution trace:

```json
{
  "status": "completed",
  "steps": [
    {"tool": "git.branch", "status": "completed", "duration_ms": 12},
    {"tool": "make.test", "status": "completed", "duration_ms": 5200},
    {"tool": "make.deploy", "status": "skipped"},
    {"tool": "slack.notify", "status": "completed", "duration_ms": 342}
  ],
  "result": "Message posted"
}
```

State is persisted to `.factorly/workflows/<run-id>.json` after each step.

See [Workflows](workflows.md) and [Expressions](expressions.md) for full documentation.

## `factorly exec` — run a command with compression + logging

Runs a single shell command through Factorly's full safety layer — the zero-config equivalent of a CLI tool definition. Uses the same code path as config-based CLI tools.

```bash
factorly exec -- git status
factorly exec --compress json -- npm test
factorly exec --env-isolation strict -- ./deploy.sh
factorly exec -i -- psql -h localhost mydb
```

Supports `{{vault:KEY}}` and `{{env:VAR}}` references in arguments — secrets stay out of shell history. Add `|default` for fallbacks:

```bash
factorly exec -- curl -H "Authorization: Bearer {{vault:GITHUB_TOKEN}}" https://api.github.com/user
factorly exec -- curl "{{env:API_URL|https://api.example.com}}/users"
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
--host <addr>        # address to bind (default: 127.0.0.1)
--port <port>        # port for HTTP mode
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

| Tool | Description | Default oversight |
|------|-------------|-------------------|
| `factorly.shell` | Run a shell command | Confirm required, destructive patterns blocked |
| `factorly.read_file` | Read a local file | Sensitive paths blocked (.env, .ssh, credentials) |
| `factorly.write_file` | Write a local file | Confirm required, system paths blocked |
| `factorly.fetch` | HTTP GET a URL | Cloud metadata + private networks blocked |
| `factorly.clipboard` | Copy text to clipboard | Confirm required |

### Context-aware

In **stdio mode** (default): all 5 tools available.
In **HTTP mode** (`--port`): only `factorly.fetch` — local tools don't make sense on a remote server.

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

When running `factorly serve --port`, you can secure the endpoint with a Bearer token. The token is resolved in order:

1. `--http-token` flag
2. `FACTORLY_HTTP_TOKEN` environment variable

Both support `{{vault:KEY}}` references — the vault is opened automatically if a vault ref is detected:

```bash
# Plain token (visible in ps output — use for dev only)
factorly serve --port 3000 --http-token mytoken

# Environment variable (better for CI/deployment)
FACTORLY_HTTP_TOKEN=mytoken factorly serve --port 3000

# Vault reference (best — token encrypted at rest)
factorly vault set HTTP_TOKEN "my-secret-token"
factorly serve --port 3000 --http-token '{{vault:HTTP_TOKEN}}'

# Vault ref via env var
FACTORLY_HTTP_TOKEN='{{vault:HTTP_TOKEN}}' factorly serve --port 3000
```

When a token is set, all HTTP requests must include `Authorization: Bearer <token>`. Requests without a valid token receive a 401 response. A warning is printed to stderr when HTTP mode starts without any token configured.

By default, the server binds to `127.0.0.1` (localhost only). If Factorly is running inside a container (Docker, devcontainer, Codespace), bind to all interfaces with `--host 0.0.0.0`:

```bash
factorly serve --host 0.0.0.0 --port 3000 --http-token '{{vault:HTTP_TOKEN}}'
factorly sync --http host.docker.internal:3000 --token '{{vault:HTTP_TOKEN}}'
```

---

[← Back to README](../README.md)
