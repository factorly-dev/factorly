# Factorly

**One MCP server. All your tools.**

Factorly wraps your existing agent tools — MCP servers, REST APIs, CLIs — into a single MCP endpoint. Configure once, connect once. Your tools don't change. Factorly just makes them all accessible from one place.

```bash
factorly serve
```

Your agent connects to Factorly and sees every tool you've configured.

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
cd factorly/src
make init
make build
```

The binary lands in `build/factorly`.

### Configure your tools

```yaml
# factorly.yaml
tools:
  slack:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-slack"]
    env:
      SLACK_TOKEN: ${SLACK_TOKEN}

  postgres:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-postgres"]
    env:
      DATABASE_URL: ${DATABASE_URL}

  hubspot:
    type: rest
    base_url: https://api.hubapi.com
    auth:
      type: bearer
      token: ${HUBSPOT_API_KEY}
    tools:
      - name: get_contacts
        method: GET
        path: /crm/v3/objects/contacts
        description: "List HubSpot contacts"
      - name: create_contact
        method: POST
        path: /crm/v3/objects/contacts
        description: "Create a new contact"

  web.fetch:
    type: cli
    description: "Fetch a webpage"
    command: curl
    args: ["-s", "{url}"]
```

Environment variables (`${VAR}`) are resolved from your shell environment or a `.env` file.

### Start Factorly

```bash
factorly serve
```

Factorly starts a single MCP server. It reads your config, connects to your tools, and exposes them all as MCP tools.

### Connect your agent

```json
// .mcp.json (Claude Code)
{
  "factorly": {
    "command": "factorly",
    "args": ["serve"]
  }
}
```

Your agent sees every tool. Credentials never leave Factorly.

## Two Ways to Use It

### As an MCP Server

```bash
factorly serve
```

Your agent connects to Factorly as a single MCP server and sees all configured tools. Best for Claude Code, Cursor, and any MCP-compatible agent.

### As a CLI

```bash
# List available tools
factorly tools

# Call a tool directly
factorly call slack.post_message --channel "#general" --text "deploy complete"

# Query your database
factorly call postgres.query --sql "SELECT count(*) FROM users"

# Fetch a webpage
factorly call web.fetch --url "https://example.com"

# Pipe output
factorly call web.fetch --url "https://example.com" | factorly call llm.summarize
```

No MCP required. Any agent, script, or automation that can shell out can use Factorly. Credentials stay secured — the caller never sees the API keys.

Works with:
- Custom Python/Node agents that use `subprocess`
- Shell scripts and cron jobs
- LangChain/CrewAI via tool definitions that call `factorly call`
- Makefiles, CI pipelines, anything that runs commands

### Both at once

`factorly serve` runs the MCP server AND the CLI works at the same time. Same config, same credentials, same log.

## What It Wraps

| Type | How It Works |
|---|---|
| **MCP servers** | Factorly spawns and manages them. Exposed via MCP and CLI. |
| **REST APIs** | Define endpoints in YAML. Exposed as callable tools via MCP and CLI. |
| **CLI commands** | Define commands in YAML. Exposed as tools via MCP and CLI. |

## What You Get

- **One endpoint** — your agent connects to Factorly, sees everything
- **Credentials secured** — API keys and tokens live in Factorly's config, not in your agent
- **Every call logged** — every tool call is logged with timestamp, parameters, and response summary
- **Zero lock-in** — your tools don't change. Remove Factorly and everything still works independently
- **Any protocol** — MCP servers, REST APIs, CLI tools. One config format for all of them

## CLI Reference

```bash
factorly serve                  # start MCP server
factorly tools                  # list all configured tools
factorly call <tool> [args]     # call a tool
factorly version                # print version
```

## Call Log

Every tool call — whether through the MCP server or the CLI — is logged to `~/.config/factorly/calls.jsonl`:

```json
{"timestamp":"2026-04-03T09:15:32Z","interface":"cli","tool":"web.fetch","params":{"url":"https://example.com"},"status":"success","duration_ms":215,"output":"<!doctype html>..."}
```

Same log regardless of how the tool was called. One source of truth.

## Config Reference

```yaml
# factorly.yaml
tools:
  <tool-name>:
    type: mcp | rest | cli        # required
    description: "..."             # optional, shown to agent

    # For MCP servers:
    command: npx                   # command to start the server
    args: ["@org/server-name"]     # arguments
    env:                           # environment variables (credentials)
      KEY: ${ENV_VAR}

    # For REST APIs:
    base_url: https://api.example.com
    auth:
      type: bearer | basic | header
      token: ${API_KEY}
    tools:
      - name: get_thing
        method: GET
        path: /v1/things
        description: "Get things"

    # For CLI commands:
    command: curl                  # executable to run
    args: ["-s", "{url}"]         # {param} placeholders are substituted
```

Parameters are inferred automatically from `{placeholder}` patterns in `args`. You can also define them explicitly:

```yaml
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{url}"]
    parameters:
      - name: url
        description: "The URL to fetch"
        required: true
```

## Development

Requires Go 1.24+.

```bash
make init       # download deps + install tooling
make build      # build for host platform → build/factorly
make test       # run all tests
make lint       # run golangci-lint
make fmt        # auto-fix lint issues + format code
make vet        # go vet
make clean      # remove build artifacts
make version    # bump patch version (BUMP=minor|major)
make release    # cross-platform binaries (linux, darwin, windows)
```

## Roadmap

- [x] Wrap CLI commands as tools
- [x] `factorly call` — CLI mode
- [x] `factorly tools` — list configured tools
- [x] Call logging (JSONL)
- [ ] `factorly serve` — MCP server mode
- [ ] Wrap MCP servers (spawn + manage child servers)
- [ ] Wrap REST APIs as tools
- [ ] `factorly test` — verify all tools are reachable
- [ ] `factorly add` — interactive tool setup
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
