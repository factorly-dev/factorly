# Config Reference

## YAML Schema

```yaml
# factorly.yaml or .factorly/factorly.yaml
tools_dir: ./tools              # optional, scan directory for tool files
disabled_commands: [vault, exec] # optional, block specific CLI commands

tools:
  <tool-name>:
    type: cli | rest | mcp      # required
    description: "..."          # optional, shown to agent

    # For CLI commands:
    command: curl               # executable to run
    args: ["-s", "{{url}}"]      # {{param}} placeholders are substituted
    stdin: "{{input}}"            # optional, pipe to subprocess stdin
    interactive: true            # optional, connect to terminal (TTY)

    # For MCP servers (stdio — spawn subprocess):
    command: npx                # executable to start the server
    args: ["@org/server-name"] # arguments
    env:                        # environment variables
      KEY: {{vault:SECRET}}
      AWS_PROFILE: "{{env:AWS_PROFILE}}"  # forward from host env
    timeout: 30s                # execution timeout (e.g. "10s", "2m"; default 30s for CLI)
    max_output: 50000           # max output bytes (default 50000)
    compress: ["all"]           # compression hints: "json", "logs", or "all"

    # For MCP servers (HTTP — connect to remote):
    url: http://host:3000/mcp  # server URL

    # For REST APIs:
    base_url: https://api.example.com
    method: GET                 # GET, POST, PUT, PATCH, DELETE
    path: /items/{{id}}           # {{param}} placeholders in path
    headers:                    # static headers (optional)
      Accept: application/json
    auth:                       # optional
      type: bearer              # bearer, basic, header, or oauth
      token: {{vault:API_KEY}}   # vault ref or {{env:ENV_VAR}}
      # header: X-Api-Key       # for header type
      # value: {{vault:KEY}}     # for header type
    parameters:
      - name: id
        in: path                # path, query, header, or body
        required: true
      - name: limit
        in: query
```

## Secret references

Use `{{env:ENV_VAR}}` for environment variables or `{{vault:KEY}}` for encrypted vault secrets. Both are resolved at startup before the agent sees anything.

## Parameter routing

Parameters are routed by their `in` field. When `in` is omitted, defaults to `query` for GET/DELETE or `body` for POST/PUT/PATCH.

## Stdin

CLI tools can pipe a parameter to the subprocess's stdin using the `stdin` field with `{{param}}` placeholders:

```yaml
tools:
  jq.filter:
    type: cli
    command: jq
    args: ["{{filter}}"]
    stdin: "{{input}}"

  clipboard.copy:
    type: cli
    description: "Copy text to the system clipboard"
    command: pbcopy        # macOS (use xclip -selection clipboard on Linux)
    stdin: "{{text}}"
```

```bash
factorly call jq.filter --filter ".name" --input '{"name":"Jordan","role":"VP Eng"}'
factorly call clipboard.copy --text "copied to clipboard"
```

## Interactive mode

CLI tools that need a TTY (database shells, SSH, REPLs) can set `interactive: true` to connect the subprocess directly to your terminal:

```yaml
tools:
  db.shell:
    type: cli
    command: psql
    args: ["-h", "localhost", "-U", "{{user}}", "{{database}}"]
    interactive: true
    env:
      PGPASSWORD: {{vault:DB_PASSWORD}}

  ssh.connect:
    type: cli
    command: ssh
    args: ["{{host}}"]
    interactive: true
```

```bash
factorly call db.shell --user admin --database myapp
# Drops into an interactive psql session with vault-injected password

factorly call ssh.connect --host prod-server
# Opens an interactive SSH session
```

Interactive tools connect stdin/stdout/stderr directly to your terminal. Output is not captured (Result is empty), but the call is still logged with tool name, params, duration, and exit code. Interactive mode only works via `factorly call`, not through `factorly serve` (MCP has no terminal).

## Output Processing

Control how tool output is compressed and truncated before it reaches the agent.

### Per-tool config

```yaml
tools:
  big-query:
    type: cli
    command: bq
    args: ["query", "{{sql}}"]
    max_output: 100000         # bytes — overrides FACTORLY_MAX_OUTPUT
    compress: ["json", "logs"] # or "all" for everything
```

### Compression pipeline

When any `compress` hint is set, ANSI escape stripping and whitespace normalization run automatically. Additional stages:

- **json** — compact JSON (strip pretty-print whitespace)
- **logs** — deduplicate repeated log lines
- **all** — enable both json and logs

### Truncation

Output exceeding `max_output` is truncated to **60% head + 40% tail** with a `[truncated]` marker in between.

### Global fallback

Set `FACTORLY_MAX_OUTPUT` to apply a default max across all tools. Per-tool `max_output` takes precedence.

### Savings tracking

Compression and truncation savings are recorded in the audit log (`original_bytes`, `processed_bytes`). Run `factorly tools status` to see an output savings summary.

## Environment Isolation

By default, child processes inherit the full parent environment (standard behavior). Set `env_isolation: strict` to restrict to a minimal set: `PATH`, `HOME`, `USER`, `LANG`, `TERM` + explicit `env:` entries only.

```yaml
tools:
  deploy:
    type: cli
    command: ./deploy.sh
    env_isolation: strict                    # minimal env (opt-in)
    env:
      AWS_PROFILE: "{{env:AWS_PROFILE}}"   # forwarded from host
      AWS_REGION: "{{env:AWS_REGION}}"      # forwarded from host
      DEPLOY_ENV: staging                    # explicit value
      API_KEY: "{{vault:DEPLOY_KEY}}"        # from encrypted vault
```

Without `env_isolation: strict`, the `env:` field adds or overrides vars on top of the full parent environment.

## Disabled Commands

Restrict which Factorly CLI commands are available. Useful for locking down access in shared or deployed environments.

```yaml
disabled_commands:
  - vault    # prevent direct vault access
  - exec     # prevent ad-hoc command execution
  - wrap     # prevent wrapping arbitrary MCP servers
```

Disabled commands return a clear error: `command "vault" is disabled in .factorly/factorly.yaml`. Commands not in the list work normally.

Supported commands: `call`, `exec`, `wrap`, `serve`, `sync`, `logs`, `vault`, `auth`.

## Parameter inference

For CLI tools, parameters are automatically inferred from `{{placeholder}}` patterns in `args` and `stdin`.

## Shadow (Governance)

Add a `shadow` block to any tool to control what agents can do:

```yaml
tools:
  github:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "{{vault:GITHUB_TOKEN}}"
    shadow:
      deny: [delete_repository, delete_branch]
      confirm: [merge_pull_request, create_release]
      rate_limit: 100/hour
      log_params: [repo, branch, title]
```

### deny

Block specific tools or sub-tools. The call is rejected immediately with a clear error.

```yaml
shadow:
  deny: [delete_repository, delete_branch]
```

For MCP servers, deny lists reference sub-tool names without the server prefix. `deny: [delete_repository]` on a `github` MCP server blocks `github.delete_repository`.

### confirm

Require human approval before execution.

- **CLI** (`factorly call`): prompts on stderr: `⚠ Tool "x" requires confirmation. Proceed? (y/n)`
- **MCP** (`factorly serve`): uses MCP elicitation — the client (Claude Code) shows the confirmation prompt in the chat UI

```yaml
# Confirm specific tools
shadow:
  confirm: [merge_pull_request, create_release]

# Confirm every call
shadow:
  confirm: true
```

### rate_limit

Limit how many times a tool can be called within a time window. Prevents runaway agents. Uses a **token bucket** algorithm for smooth throttling — no window-boundary bursts.

```yaml
shadow:
  rate_limit: 100/hour    # 100 calls per hour
  rate_limit: 10/min      # 10 calls per minute
  rate_limit: 5/sec       # 5 calls per second
```

Rate limit state persists across `factorly call` invocations (stored at `~/.config/factorly/ratelimit.json`). Reset by deleting that file.

### Loop detection

Always-on — no configuration needed. Factorly fingerprints identical calls (same tool + same parameters) within a **300-second sliding window** and applies three tiers:

| Repeat count | Behavior |
|---|---|
| 1–3 | Normal execution |
| 4–11 | Warning logged, execution continues |
| 12+ | Call blocked |

### log_params

Highlight specific parameters in the call log for audit visibility.

```yaml
shadow:
  log_params: [repo, branch, title]
```

These params appear in the `highlight_params` field of the JSONL log entry, making it easy to search and filter audit trails.

### Per-tool shadow

Shadow works on any tool type — CLI, REST, or MCP:

```yaml
tools:
  stripe.charge:
    type: rest
    base_url: https://api.stripe.com
    method: POST
    path: /v1/charges
    auth:
      type: bearer
      token: "{{vault:STRIPE_KEY}}"
    shadow:
      confirm: true
      rate_limit: 10/min
```

---

[← Back to README](../README.md)
