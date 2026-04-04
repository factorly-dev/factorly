# Factorly

**One MCP server. All your tools.**

Factorly wraps your existing agent tools — MCP servers, REST APIs, CLIs — into a single MCP endpoint. Configure once, connect once. Your tools don't change. Factorly just makes them all accessible from one place.

```bash
factorly init
factorly tools
factorly call web.fetch --url "https://example.com"
```

## Why

You're building an AI agent. It needs to talk to Slack, query a database, hit an API, read files. Each tool has its own credentials, its own connection, its own auth. You end up with:

- API keys scattered across `.env` files
- MCP servers configured individually in every project
- Auth logic duplicated everywhere
- No log of what your agent actually called
- Key rotation means hunting through configs

Factorly fixes this. One config file. One endpoint. Every tool.

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
      token: ${GITHUB_TOKEN}
    parameters:
      - name: username
        in: path
        required: true
      - name: per_page
        in: query
```

Environment variables (`${VAR}`) are resolved from your shell environment or a `.env` file.

### Use it

```bash
# List available tools
factorly tools

# Call a CLI tool
factorly call web.fetch --url "https://example.com"

# Call a REST API
factorly call github.repos --username octocat --per_page 5

# Verbose mode — see what's happening
factorly -v call web.fetch --url "https://example.com"
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
    token: "${SLACK_TOKEN}"
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
- **Credentials secured** — API keys and tokens live in Factorly's config, not in your agent
- **Every call logged** — every tool call is logged with timestamp, parameters, and response summary
- **Zero lock-in** — your tools don't change. Remove Factorly and everything still works independently
- **Any protocol** — REST APIs, CLI tools, and soon MCP servers. One config format for all of them

## CLI Reference

```bash
factorly init                       # create .factorly/factorly.yaml (interactive)
factorly init --out factorly.yaml   # create at custom path
factorly tools                      # list all configured tools
factorly call <tool> [--param val]  # call a tool
factorly import openapi <spec>      # generate tools from OpenAPI spec
factorly version                    # print version
```

**Global flags:**

```bash
-v, --verbose       # print debug info to stderr
-c, --config <path> # path to factorly.yaml
    --config-dir    # load tools from a directory (no config file needed)
```

## Call Log

Every tool call is logged to `~/.config/factorly/calls.jsonl`:

```json
{"timestamp":"2026-04-03T09:15:32Z","interface":"cli","tool":"web.fetch","params":{"url":"https://example.com"},"status":"success","duration_ms":215,"output":"<!doctype html>..."}
```

Set `FACTORLY_NO_LOG=1` to disable logging.

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

    # For REST APIs:
    base_url: https://api.example.com
    method: GET                 # GET, POST, PUT, PATCH, DELETE
    path: /items/{id}           # {param} placeholders in path
    headers:                    # static headers (optional)
      Accept: application/json
    auth:                       # optional
      type: bearer              # bearer, basic, or header
      token: ${API_KEY}         # for bearer
      # header: X-Api-Key       # for header type
      # value: ${API_KEY}       # for header type
    parameters:
      - name: id
        in: path                # path, query, header, or body
        required: true
      - name: limit
        in: query
```

**Parameter routing:** Parameters are routed by their `in` field. When `in` is omitted, defaults to `query` for GET/DELETE or `body` for POST/PUT/PATCH.

**Parameter inference:** For CLI tools, parameters are automatically inferred from `{placeholder}` patterns in `args`.

## Development

Requires Go 1.24+.

```bash
make init               # download deps + install tooling (golangci-lint, gotestsum)
make build              # build for host platform → build/factorly
make test               # run unit + integration tests
make test-unit          # unit tests only
make test-integration   # integration tests only (builds binary first)
make lint               # run golangci-lint
make fmt                # auto-fix lint issues + format code
make vet                # go vet
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
- [x] Call logging (JSONL)
- [x] `--verbose` flag
- [ ] `factorly serve` — MCP server mode
- [ ] MCP provider — spawn + manage child MCP servers
- [ ] `factorly test` — verify all tools are reachable
- [ ] `factorly logs` — view/query the call log
- [ ] Tool health checks
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
