# Getting Started

## Install

```bash
npm install -g @factorly/cli
```

Or with Go:

```bash
go install github.com/factorly-dev/factorly-cli@latest
```

Or build from source:

```bash
git clone https://github.com/factorly-dev/factorly-cli.git
cd factorly-cli && make build
# binary at build/factorly — add to PATH or move to /usr/local/bin
```

## Try It Immediately

No config needed — wrap any existing MCP server:

```bash
factorly wrap -- npx @modelcontextprotocol/server-everything
```

Or install a pre-built template:

```bash
factorly tools import templates github
factorly call github.list_repos --username octocat
```

## Set Up a Project

```bash
# Interactive setup — creates .factorly/factorly.yaml
factorly init

# Store a secret
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# Check everything works
factorly tools status
```

## Add Tools

**From a template** (easiest):

```bash
factorly tools import templates
# Shows 36 available templates — pick one
factorly tools import templates slack
```

**Interactively:**

```bash
factorly tools add
```

**From an OpenAPI spec:**

```bash
factorly tools import openapi ./api-spec.yaml --out .factorly/tools/api.yaml
```

**From a curl command:**

```bash
echo 'curl -H "Authorization: Bearer $TOKEN" https://api.example.com/data' | factorly tools record
```

## Connect to Your Agent

Auto-detect installed AI clients and write their MCP config:

```bash
factorly sync
```

This writes the Factorly MCP server entry into `.mcp.json` (Claude Code), `.cursor/mcp.json` (Cursor), and `.codex/mcp.json` (Codex) — whichever are detected.

### Manual setup

**Claude Code** — add to `.mcp.json` in your project root:

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

**Cursor** — add to `.cursor/mcp.json` with the same format.

**OpenAI Codex** — add to `.codex/mcp.json` with the same format.

### HTTP mode

For remote or shared servers:

```bash
factorly serve --http :3000
```

Endpoint at `http://localhost:3000/mcp`. Secure with `--http-token` or `FACTORLY_HTTP_TOKEN`. See [CLI Reference](cli-reference.md) for details.

> **Note:** If running inside Docker, use `host.docker.internal` instead of `localhost`. See [CLI Reference](cli-reference.md#http-server-authentication) for container setup.

## Next Steps

- [Config Reference](config-reference.md) — full YAML schema
- [Templates](templates.md) — 36 pre-built service configs
- [Vault](vault.md) — encrypted secret storage
- [OAuth](oauth.md) — authenticate with Google, GitHub, Microsoft

---

[← Back to Documentation](README.md)
