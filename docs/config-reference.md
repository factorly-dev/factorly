# Config Reference

## YAML Schema

```yaml
# factorly.yaml or .factorly/factorly.yaml
tools_dir: ./tools               # optional, scan directory for tool files
disabled_commands: [vault, exec] # optional, block specific CLI commands
disable_builtins: true           # optional, disable all factorly.* built-in tools

vault_backends:                  # optional, external secret managers
  op:                            # backend name → use as {{op:KEY}}
    type: cli
    get:
      command: op
      args: ["read", "op://Development/{{key}}"]
    list:                        # optional
      command: op
      args: ["item", "list", "--format=json"]


oauth_providers: # shared across tools
  google:
    client_id: "{{vault:GOOGLE_CLIENT_ID}}"
    client_secret: "{{vault:GOOGLE_CLIENT_SECRET}}"
    auth_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    scopes: ["https://www.googleapis.com/auth/drive.readonly"]

tools:
  <tool-name>:
    type: cli | rest | mcp | workflow  # required
    description: "..."                 # optional, shown to agent

    # For CLI commands:
    command: curl                # executable to run
    args: ["-s", "{{url}}"]      # {{param}} placeholders are substituted
    stdin: "{{input}}"           # optional, pipe to subprocess stdin
    interactive: true            # optional, connect to terminal (TTY)

    # For MCP servers (stdio — spawn subprocess):
    command: npx                 # executable to start the server
    args: ["@org/server-name"]   # arguments
    env:                         # environment variables
      KEY: {{vault:SECRET}}
      HOME: "{{env:HOME}}"       # forward from host env
    timeout: 30s                 # execution timeout (e.g. "10s", "2m"; default 30s for CLI)
    max_output: 50000            # max output bytes (default 50000)
    compress: ["all"]            # compression hints: "json", "logs", or "all"

    # For MCP servers (HTTP — connect to remote):
    url: http://host:3000/mcp    # server URL

    # For REST APIs:
    base_url: https://api.example.com
    method: GET                  # GET, POST, PUT, PATCH, DELETE
    path: /items/{{id}}          # {{param}} placeholders in path
    body: |                      # optional, JSON body template with {{param}} placeholders
      {"model":"{{model}}","messages":[{"role":"user","content":"{{prompt}}"}]}
    headers:                     # static headers (optional)
      Accept: application/json
    auth:                        # optional
      type: bearer               # bearer, basic, header, or oauth
      token: {{vault:API_KEY}}   # for bearer
      # header: X-Api-Key        # for header type
      # value: {{vault:KEY}}     # for header type

      # OAuth (inline):
      # type: oauth
      # client_id: {{vault:CLIENT_ID}}
      # client_secret: {{vault:CLIENT_SECRET}}
      # auth_url: https://provider.com/oauth/authorize
      # token_url: https://provider.com/oauth/token
      # scopes: ["read", "write"]
      # token_key: provider_oauth    # vault key for token bundle

      # OAuth (provider reference):
      # type: oauth
      # provider: google             # references oauth_providers section

    env_isolation: strict            # optional, restrict child env to essentials
    parameters:
      - name: id
        in: path                     # path, query, header, or body
        required: true
      - name: limit
        in: query
        type: integer                # string (default), integer, number, boolean, json
        default: 100                 # default value if not provided by caller
        min: 1                       # clamp to minimum (integers and numbers)
        max: 100                     # clamp to maximum (integers and numbers)
      - name: state
        type: string
        enum: [open, closed, all]    # reject values not in list
      - name: owner
        type: string
        pattern: "^[a-zA-Z0-9-]+$"  # reject values not matching regex
        min_length: 1                # reject strings shorter than this
        max_length: 39               # truncate strings longer than this

    # For workflows (sequential tool pipelines):
    steps:
      - tool: github.list_repos       # tool to call (must exist in config)
        params: { owner: "{{org}}" }  # {{var}} substituted from input + stored outputs
        store: repos                   # save output as variable for later steps
      - tool: anthropic.ask
        params:
          prompt: "Summarize: {{repos}}"

    shadow:                          # optional, oversight rules
      deny: [dangerous_action]
      confirm: true                  # or ["specific_action"]
      rate_limit: 100/hour
      log_params: [id]
      allow_patterns: []             # override denied shell patterns (built-in tools)
      allow_paths: []                # override denied file paths (built-in tools)
      allow_urls: []                 # override denied URLs (built-in tools)

    filter:                          # optional, command-aware output filtering
      match_output:                  # short-circuit: replace output with message
        - pattern: "Build complete"
          message: "ok (build succeeded)"
          unless: "error|Error|FAIL"
      strip_lines:                   # drop lines matching any regex
        - "^\\s*$"
        - "^\\[INFO\\]"
      keep_lines:                    # keep only matching lines (mutually exclusive with strip_lines)
        - "^FAIL"
        - "^PASS"
      replace:                       # regex substitutions (applied in order)
        - pattern: "/home/\\w+"
          replacement: "~"
      head_lines: 10                 # keep first N lines
      tail_lines: 5                  # keep last N lines (inserts omission marker)
      max_lines: 50                  # absolute line count cap
      json_path: "$.content[0].text" # JSONPath extraction (full spec)
      pipe:                          # pipe output through external tool (cli, rest, mcp)
        type: cli                  # cli (default), rest, or mcp
        command: jq
        args: [".items[:10]"]
        timeout: 5s

```

## Secret references

Use `{{env:ENV_VAR}}` for environment variables or `{{vault:KEY}}` for encrypted vault secrets. Both are resolved at startup before the agent sees anything.

### Default values

Add a `|` after the key to specify a fallback if the value is not found:

```yaml
tools:
  api.call:
    type: rest
    base_url: "{{env:API_URL|https://api.example.com}}"
    method: GET
    path: /items
    auth:
      type: bearer
      token: "{{vault:API_KEY|sk-default-dev-key}}"
```

Defaults work with all reference types:

```yaml
# Vault — use fallback if key not in vault
token: "{{vault:TOKEN|default-value}}"

# Environment — use fallback if env var not set
home: "{{env:HOME|/tmp}}"

# External backends — use fallback if key not found
secret: "{{op:vault/item|fallback}}"

# Parameters — use fallback if param not provided by caller
args: ["--limit", "{{limit|100}}", "--host", "{{host|localhost}}"]
```

If no default is specified and the value is missing, Factorly returns an error.

To pass a literal `{{...}}` without resolution, escape with a backslash: `\{{vault:TOKEN}}` passes through as `{{vault:TOKEN}}`.

## Parameter routing

Parameters are routed by their `in` field. When `in` is omitted, defaults to `query` for GET/DELETE or `body` for POST/PUT/PATCH.

### Parameter types

The `type` field controls how body parameters are serialized in JSON:

| Type | JSON output | Example |
|------|------------|---------|
| `string` (default) | Quoted | `"claude-sonnet-4-20250514"` |
| `integer` | Unquoted number | `1024` |
| `number` | Unquoted number | `0.95` |
| `boolean` | Unquoted | `true` |
| `json` | Raw passthrough | `[{"role":"user","content":"hello"}]` |

When multiple parameters have `in: body`, they are merged into a single JSON object using their types for serialization.

### Parameter validation

Parameters can have validation rules that catch broken calls before execution. Where possible, values are coerced (fixed) rather than rejected — the audit log records what was changed.

| Rule | Applies to | Behavior |
|------|-----------|----------|
| `min` / `max` | `integer`, `number` | Clamp to boundary (coerce) |
| `min_length` | `string` | Reject if too short |
| `max_length` | `string` | Truncate if too long (coerce) |
| `pattern` | `string` | Reject if regex doesn't match |
| `enum` | `string` | Reject if not in allowed list |

**Type validation** is always active when `type` is set:
- `integer` — must parse as int; floats like `"3.0"` are truncated to `3`
- `number` — must parse as float
- `boolean` — `"true"/"1"/"yes"` → `"true"`, `"false"/"0"/"no"` → `"false"`
- `json` — must be valid JSON

**Coercion vs rejection:** Min/max clamping, max_length truncation, and type normalization are coerced silently. Pattern, enum, min_length, and unparseable types are rejected with an error returned to the caller (the LLM gets the error and can retry).

**Audit trail:** When parameters are coerced, the log entry includes `original_params` with pre-coercion values and `shadow_action: "modified"`. Rejected calls log `shadow_action: "invalid"`.

```yaml
parameters:
  - name: temperature
    type: number
    min: 0
    max: 2
    default: 0.7
  - name: max_tokens
    type: integer
    min: 1
    max: 4096
  - name: format
    type: string
    enum: [json, text, csv]
  - name: query
    type: string
    pattern: "^[a-zA-Z0-9 ]+$"
    max_length: 500
```

### Body template

For REST tools with complex JSON bodies, use the `body` field instead of individual body parameters. Placeholders are substituted directly:

```yaml
tools:
  anthropic.ask:
    type: rest
    base_url: "https://api.anthropic.com"
    method: POST
    path: /v1/messages
    body: |
      {"model":"{{model}}","max_tokens":{{max_tokens}},"messages":[{"role":"user","content":"{{prompt}}"}]}
    parameters:
      - name: prompt
        description: The question for Claude
        required: true
      - name: model
        default: claude-sonnet-4-20250514
      - name: max_tokens
        default: "1024"
```

```bash
factorly call anthropic.ask --prompt "What is the capital of France?"
```

When `body` is set, parameter `in` fields are ignored for body routing — all substitution happens in the template.

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

Control how tool output is compressed, filtered, and truncated before it reaches the agent.

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

### Processing pipeline

Output passes through these stages in order:

1. **ANSI strip** — always on, removes color/formatting escape codes
2. **Whitespace normalize** — always on, collapses 3+ blank lines to 2
3. **Filter** — per-tool, command-aware filtering (see below)
4. **JSON compact** — when `compress` includes `json` or `all`
5. **Log dedup** — when `compress` includes `logs` or `all`
6. **Byte truncation** — when `max_output` is set, 60% head + 40% tail

### Filters

Per-tool output filters for command-aware compression — strip noise lines, short-circuit verbose output to summaries, regex replace, and line-based truncation. Factorly ships with 27 built-in filters for common commands (git, make, npm, go, cargo, pytest, pip, apt, brew, docker, kubectl, terraform) that apply automatically.

See [Output Filters](filters.md) for the full filter schema, stages, and built-in filter list.

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

## Shadow (Oversight)

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
