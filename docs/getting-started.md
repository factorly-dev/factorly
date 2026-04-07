# Getting Started

## Install

```bash
git clone https://github.com/factorly-hq/factorly-cli.git
cd factorly
make init
make build
```

The binary lands in `build/factorly`. Add it to your PATH or move it to `/usr/local/bin`.

## Initialize a project

```bash
factorly init
```

This creates `.factorly/factorly.yaml` with an interactive setup — optionally adds a tools directory, an example CLI tool, and can import tools from an OpenAPI spec.

Use `--out factorly.yaml` to write to the project root instead.

## Add your first tool

```bash
# Interactive
factorly tools add

# Or non-interactive
factorly tools add --name web.fetch --type cli --command curl --args '-s,{url}'
```

## Use it

```bash
# List available tools
factorly tools

# Call a tool
factorly call web.fetch --url "https://example.com"

# Check everything is working
factorly status
```

## Connect to your agent

The fastest way — auto-detect installed clients and write their config:

```bash
factorly sync
```

This writes the Factorly MCP server entry into `.mcp.json` (Claude Code), `.cursor/mcp.json` (Cursor), and `.codex/mcp.json` (Codex) — whichever are detected.

### Manual setup

If you prefer to configure manually:

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

**Cursor** — add to `.cursor/mcp.json`:

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

**OpenAI Codex** — add to `.codex/mcp.json`:

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

### HTTP mode

For remote or shared servers:

```bash
factorly serve --http :3000
```

Endpoint at `http://localhost:3000/mcp`. Secure with `--http-token` or `FACTORLY_HTTP_TOKEN`. See [CLI Reference](cli-reference.md) for details.

Sync HTTP mode to clients:

```bash
factorly sync --http localhost:3000 --token mytoken
```

> **Note:** If running inside Docker, use `host.docker.internal` instead of `localhost`. See [CLI Reference](cli-reference.md#http-server-authentication) for container setup.

## Store secrets

```bash
# Store a secret in the encrypted vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx

# Reference it in your config
# token: "${vault:GITHUB_TOKEN}"
```

See [Vault](vault.md) for full documentation.

## Next steps

- [Config Reference](config-reference.md) — full YAML schema
- [OAuth](oauth.md) — authenticate with Google, GitHub, Microsoft
- [OpenAPI Import](openapi-import.md) — generate tools from API specs

---

[← Back to Documentation](README.md)
