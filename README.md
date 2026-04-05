# Factorly

**Your agent calls tools. Factorly holds the keys.**

Factorly wraps your existing agent tools — REST APIs, CLIs, MCP servers — into a single endpoint where credentials never reach the agent. Your agent sees tool names and parameters. Factorly injects the auth, makes the call, and returns the data.

```bash
# Your agent runs this — no secrets anywhere in the command
factorly call github.repos --username octocat --per_page 5

# Factorly injects the token, makes the HTTP call, returns the data
# The agent never sees GITHUB_TOKEN
```

## Why

Your AI agent needs to call Slack, GitHub, Stripe, a database, an internal API. Today that means:

- **Secrets in the agent's context** — every API key is one prompt injection away from exposure
- **Auth logic duplicated** across every tool, every project
- **No audit trail** — what did the agent actually call? With what parameters?
- **Key rotation** means hunting through agent configs and .env files

Factorly fixes this. Secrets live in Factorly's config — or encrypted in the vault. The agent only knows tool names.

```
┌──────────────────────┐         ┌──────────────────────────────┐
│  Your Agent          │         │  Factorly                    │
│                      │         │                              │
│  Knows:              │  call   │  Injects:                    │
│  - tool names        │────────▶│  - Authorization headers     │
│  - parameter names   │         │  - API keys from vault       │
│                      │◀────────│  - Base URLs                 │
│  Never sees:         │  data   │                              │
│  - API keys          │         │  Logs every call.            │
│  - tokens            │         │  Returns only data.          │
│  - credentials       │         │                              │
└──────────────────────┘         └──────────────────────────────┘
```

## Quick Start

### Install from source

```bash
git clone https://github.com/factorly-dev/factorly.git
cd factorly
make init
make build
```

The binary lands in `build/factorly`.

### Initialize a project

```bash
factorly init
```

This creates `.factorly/factorly.yaml` with an interactive setup — optionally adds a tools directory, an example CLI tool, and can import tools from an OpenAPI spec.

Use `--out factorly.yaml` to write to the project root instead.

### Configure your tools

```yaml
# .factorly/factorly.yaml
tools:
  web.fetch:
    type: cli
    description: "Fetch a webpage"
    command: curl
    args: ["-s", "{url}"]

  github.repos:
    type: rest
    description: "List repos for a user"
    base_url: https://api.github.com
    method: GET
    path: /users/{username}/repos
    auth:
      type: bearer
      token: "${vault:GITHUB_TOKEN}"
    parameters:
      - name: username
        in: path
        required: true
      - name: per_page
        in: query
```

### Use it

```bash
# List available tools
factorly tools

# Call a CLI tool
factorly call web.fetch --url "https://example.com"

# Call a REST API
factorly call github.repos --username octocat --per_page 5

# Start as an MCP server (stdio)
factorly serve

# Start as an HTTP MCP server (endpoint: http://localhost:3000/mcp)
factorly serve --http :3000

# Verbose mode — see what's happening
factorly -v call web.fetch --url "https://example.com"
```

## Connect to Your Agent

### Claude Code

Add to `.mcp.json` in your project root:

```json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

Claude Code starts Factorly as a subprocess via stdio. All configured tools appear automatically.

For HTTP mode (remote or shared server):

```json
{
  "mcpServers": {
    "factorly": {
      "type": "streamable-http",
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

Or for HTTP mode:

```json
{
  "mcpServers": {
    "factorly": {
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

### OpenAI Codex

Add to `.codex/mcp.json`:

```json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

### Any MCP Client

Factorly supports two transports:

- **Stdio** (default) — `factorly serve` — the client starts Factorly as a subprocess. Best for local development.
- **Streamable HTTP** — `factorly serve --http :3000` — Factorly runs as a standalone server at `http://localhost:3000/mcp`. Best for shared/remote/hosted setups.

## Vault — Encrypted Secret Storage

Factorly includes an encrypted vault so secrets never live in plaintext `.env` files. Two-layer encryption: the vault file is encrypted with AES-256-GCM (Argon2id-derived key), and each secret value is independently encrypted with its own salt via HKDF-SHA256.

This means `vault list` never decrypts secret values, and `vault get` only decrypts the one you ask for. Identical values stored under different keys produce different ciphertext.

### Store and retrieve secrets

```bash
# Interactive — prompts for value with no echo
factorly vault set GITHUB_TOKEN

# Inline
factorly vault set STRIPE_KEY sk_live_xxxxxxxxxxxx

# Retrieve a secret (outputs raw value, pipe-friendly)
factorly vault get GITHUB_TOKEN

# List stored keys (values stay encrypted)
factorly vault list

# Remove a secret
factorly vault delete OLD_KEY
```

### Reference in config

Use `${vault:KEY}` anywhere you'd use `${ENV_VAR}`:

```yaml
# Env var (plain text in environment)
token: "${GITHUB_TOKEN}"

# Vault (encrypted on disk, decrypted on demand)
token: "${vault:GITHUB_TOKEN}"
```

Both work. Mix and match per tool. Vault references are resolved at call time — the agent never sees either form.

### How it works

```
factorly vault set GITHUB_TOKEN
  → prompts for value (no echo)
  → derives per-entry key (master key + random salt → HKDF-SHA256)
  → encrypts value with AES-256-GCM
  → stores encrypted entry in vault index
  → re-encrypts vault file to disk

factorly call github.repos --username octocat
  → loads config, finds ${vault:GITHUB_TOKEN}
  → decrypts vault index (key names + encrypted blobs)
  → decrypts only the requested entry on demand
  → injects into Authorization header
  → agent sees only the response data
```

### Encryption details

| Layer | Algorithm | Key derivation | Purpose |
|-------|-----------|---------------|---------|
| File | AES-256-GCM | Password → Argon2id (128MB, 2 iterations) → master key | Protects key names + encrypted entry blobs |
| Per-entry | AES-256-GCM | Master key + 16-byte random salt → HKDF-SHA256 → entry key | Protects each secret value independently |

Each entry has its own random salt and nonce, regenerated on every write. Entry keys are zeroized immediately after use. The master key is zeroized on `Close()`.

### Vault password

The vault is locked with a master password. Resolved in order:

1. `FACTORLY_VAULT_PASSWORD` env var (CI/automation)
2. `~/.config/factorly/vault.key` file (headless servers)
3. Interactive prompt (normal dev UX)

### Vault path

Default: `~/.config/factorly/vault.enc`. Override with `--vault-path` flag or `FACTORLY_VAULT_PATH` env var.

### Migration

Existing vaults are automatically migrated to per-entry encryption on first open. No action needed — the upgrade is transparent and preserves all stored values.

### Extensible backends (future)

The vault uses a `Backend` interface. The local encrypted file is the default backend. Future backends:

```yaml
# Coming soon
token: "${1password:Development/GitHub/token}"
token: "${gcp-sm:project-id/GITHUB_TOKEN}"
token: "${aws-sm:prod/stripe-key}"
```

## Import from OpenAPI

Generate tool definitions from any OpenAPI 3.x spec — local file or remote URL:

```bash
# From a URL
factorly import openapi https://petstore3.swagger.io/api/v3/openapi.json --out .factorly/tools/petstore.yaml

# From a local file
factorly import openapi ./api-spec.yaml --out .factorly/tools/api.yaml

# With a custom prefix
factorly import openapi ./spec.yaml --prefix myapi

# Preview to stdout
factorly import openapi ./spec.yaml
```

Each operation becomes a REST tool with method, path, parameters (query/path/header/body), auth, and base URL extracted automatically.

## Project Directory

Factorly supports a `.factorly/` project directory for per-repo tool configs — similar to `.github/`, `.vscode/`, or `.cursor/`:

```
my-project/
├── .factorly/
│   ├── factorly.yaml        # project config (optional)
│   ├── tools/               # modular tool files (optional)
│   │   ├── slack.yaml
│   │   └── github.yaml
│   └── petstore.yaml        # loose tool files work too
├── factorly.yaml            # top-level config (optional, merges with .factorly/)
└── ...
```

**How it loads:**

1. Top-level `factorly.yaml` in cwd (if present, merges `.factorly/`)
2. `.factorly/factorly.yaml` (if no top-level config)
3. Loose YAML files in `.factorly/` (if no factorly.yaml inside it)
4. `~/.config/factorly/factorly.yaml` (user-level fallback)

Each YAML file in a tools directory is a flat map of tool definitions — no wrapper key needed:

```yaml
# .factorly/tools/slack.yaml
slack.post:
  type: rest
  base_url: https://slack.com/api
  method: POST
  path: /chat.postMessage
  auth:
    type: bearer
    token: "${vault:SLACK_TOKEN}"
  parameters:
    - name: channel
      required: true
    - name: text
      required: true
```

Use `tools_dir` in your config to point to a tools directory:

```yaml
# .factorly/factorly.yaml
tools_dir: ./tools
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{url}"]
```

## What It Wraps

| Type | How It Works | Status |
|---|---|---|
| **CLI commands** | Define command + args in YAML. `{param}` placeholders substituted. | Working |
| **REST APIs** | Define base URL, method, path, auth, parameters. HTTP calls with routing. | Working |
| **MCP servers** | Spawn and manage child servers. Forward calls. | Planned |

## What You Get

- **One endpoint** — your agent connects to Factorly, sees everything
- **Credentials secured** — secrets live in the encrypted vault or env vars, never in the agent
- **Every call logged** — every tool call is logged with timestamp, parameters, and response summary
- **Zero lock-in** — your tools don't change. Remove Factorly and everything still works independently
- **Any protocol** — REST APIs, CLI tools, and soon MCP servers. One config format for all of them

## CLI Reference

```bash
factorly serve                      # start MCP server (stdio)
factorly serve --http :3000         # start MCP server (HTTP at /mcp)
factorly init                       # create .factorly/factorly.yaml (interactive)
factorly init --out factorly.yaml   # create at custom path
factorly tools                      # list all configured tools
factorly call <tool> [--param val]  # call a tool
factorly import openapi <spec>      # generate tools from OpenAPI spec
factorly vault set <key> [value]    # store a secret (prompts if no value)
factorly vault get <key>            # retrieve a secret (raw value to stdout)
factorly vault list                 # list secret names
factorly vault delete <key>         # remove a secret
factorly version                    # print version
```

**Global flags:**

```bash
-v, --verbose          # print debug info to stderr
-c, --config <path>    # path to factorly.yaml
    --config-dir       # load tools from a directory (no config file needed)
```

**Vault flags:**

```bash
    --vault-path       # path to vault file (default: ~/.config/factorly/vault.enc)
```

**Environment variables:**

```bash
FACTORLY_VAULT_PASSWORD   # vault master password (for CI/automation)
FACTORLY_VAULT_PATH       # vault file path override
FACTORLY_NO_LOG           # disable call logging when set
```

## Call Log

Every tool call is logged to `~/.config/factorly/calls.jsonl`:

```json
{"timestamp":"2026-04-03T09:15:32Z","interface":"cli","tool":"web.fetch","params":{"url":"https://example.com"},"status":"success","duration_ms":215,"output":"<!doctype html>..."}
```

## Config Reference

```yaml
# factorly.yaml or .factorly/factorly.yaml
tools_dir: ./tools              # optional, scan directory for tool files

tools:
  <tool-name>:
    type: cli | rest            # required
    description: "..."          # optional, shown to agent

    # For CLI commands:
    command: curl               # executable to run
    args: ["-s", "{url}"]      # {param} placeholders are substituted
    stdin: "{input}"            # optional, pipe to subprocess stdin

    # For REST APIs:
    base_url: https://api.example.com
    method: GET                 # GET, POST, PUT, PATCH, DELETE
    path: /items/{id}           # {param} placeholders in path
    headers:                    # static headers (optional)
      Accept: application/json
    auth:                       # optional
      type: bearer              # bearer, basic, or header
      token: ${vault:API_KEY}   # vault ref or ${ENV_VAR}
      # header: X-Api-Key       # for header type
      # value: ${vault:KEY}     # for header type
    parameters:
      - name: id
        in: path                # path, query, header, or body
        required: true
      - name: limit
        in: query
```

**Secret references:** Use `${ENV_VAR}` for environment variables or `${vault:KEY}` for encrypted vault secrets. Both are resolved at startup before the agent sees anything.

**Parameter routing:** Parameters are routed by their `in` field. When `in` is omitted, defaults to `query` for GET/DELETE or `body` for POST/PUT/PATCH.

**Stdin:** CLI tools can pipe a parameter to the subprocess's stdin using the `stdin` field with `{param}` placeholders:

```yaml
tools:
  jq.filter:
    type: cli
    command: jq
    args: ["{filter}"]
    stdin: "{input}"

  clipboard.copy:
    type: cli
    description: "Copy text to the system clipboard"
    command: pbcopy        # macOS (use xclip -selection clipboard on Linux)
    stdin: "{text}"
```

```bash
factorly call jq.filter --filter ".name" --input '{"name":"Jordan","role":"VP Eng"}'
factorly call clipboard.copy --text "copied to clipboard"
```

**Parameter inference:** For CLI tools, parameters are automatically inferred from `{placeholder}` patterns in `args`.

## Examples

- **[Secure Agent](src/examples/secure-agent/)** — Full example showing how Factorly keeps secrets out of your agent's context. GitHub, Slack, and Stripe APIs configured with modular tool files. The agent calls tools by name — credentials are injected by Factorly and never exposed.

## Development

Requires Go 1.24+.

```bash
make init               # download deps + install tooling (golangci-lint, gotestsum)
make build              # build for host platform → build/factorly
make test               # run unit + integration tests
make test-unit          # unit tests only
make test-integration   # integration tests only (builds binary first)
make ci                 # full CI pipeline: tidy, fmt, vet, lint, test
make lint               # run golangci-lint
make fmt                # auto-fix lint issues + format code
make vet                # go vet
make tidy               # go mod tidy
make clean              # remove build artifacts
make version            # bump patch version (BUMP=minor|major)
make release            # cross-platform binaries (linux, darwin, windows)
```

## Roadmap

- [x] CLI provider — wrap shell commands as tools
- [x] REST provider — wrap HTTP APIs as tools
- [x] `factorly call` — call any tool from the CLI
- [x] `factorly tools` — list configured tools
- [x] `factorly init` — interactive project setup
- [x] `factorly import openapi` — generate tools from OpenAPI specs (local + remote)
- [x] Tool directory — modular configs via `tools_dir` and `.factorly/`
- [x] Encrypted vault — `${vault:KEY}` with per-entry encryption (HKDF + AES-256-GCM)
- [x] `factorly serve` — MCP server mode (stdio + HTTP)
- [x] Call logging (JSONL)
- [x] `--verbose` flag
- [ ] MCP provider — spawn + manage child MCP servers
- [ ] `factorly doctor` — health check all tools, credentials, and connections
- [ ] `factorly sync` — push MCP config into AI clients (Claude Code, Cursor, Codex)
- [ ] `factorly status` — overview of tools, synced clients, connection health
- [ ] `factorly logs` — view/query the call log
- [ ] External vault backends (1Password, GCP Secret Manager, AWS)
- [ ] Hosted version (Factorly Cloud)
- [ ] Team configs and shared credential vault
- [ ] Dashboard and audit log UI

## Philosophy

- **Wrap, don't replace.** Your tools don't change. Factorly sits in front.
- **Zero lock-in.** Remove Factorly and everything still works.
- **Credentials are not your agent's business.** Secrets live in Factorly, not in your agent.
- **Log everything.** If an agent did it, there's a record.

## License

MIT
