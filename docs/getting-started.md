# Getting Started

## Install

```bash
brew install factorly-dev/tap/factorly
```

```bash
npm install -g factorly
```

```bash
pip install factorly
```

```bash
go install github.com/factorly-dev/factorly@latest
```

Or build from source:

```bash
git clone https://github.com/factorly-dev/factorly.git
cd factorly && make build
# binary at build/factorly — add to PATH or move to /usr/local/bin
```

## Try It Immediately

Zero config: wrap any existing MCP server and it's instantly callable through Factorly:

```bash
factorly wrap -- npx @modelcontextprotocol/server-everything
```

That's the fastest way to see Factorly working. The [`wrap`](cli-reference.md)
command needs no `.factorly/` directory and no setup.

## Your First Real Tool (the happy path)

One uninterrupted thread from nothing to "it's in my agent":

```bash
# 1. Create a project config
factorly init

# 2. Install a ready-made tool bundle (41 services available)
factorly blueprint install github

# 3. Give it the credential it needs
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxx
#    (some services use OAuth instead — see "When a blueprint needs OAuth" below)

# 4. Confirm it's wired up — names any missing keys / auth
factorly tools status

# 5. Fire a real call to prove it works
factorly call github.list_repos --username octocat

# 6. Connect it to your agent (auto-detects Claude Code, Cursor, Codex)
factorly sync
#    then reload/restart your agent — Factorly's tools appear in its tool list
```

If step 5 fails with an unresolved `{{vault:KEY}}` error, the message
names the exact key and the command to fix it. Run `factorly tools status`
anytime to see which tools are healthy and which need credentials.

Why a vault instead of an env var? Your agent never receives `GITHUB_TOKEN` — Factorly
resolves the `{{vault:GITHUB_TOKEN}}` reference into the outbound request, and the
agent only sees GitHub's response. See [Vault](vault.md) for the full model.

### When a blueprint needs OAuth

Some services (Google, Microsoft, GitHub Apps) authenticate via OAuth
rather than a static token. Instead of `vault set`, run:

```bash
factorly auth login github
```

See [OAuth](oauth.md) for the full flow.

## Add Tools

**From a blueprint** (easiest):

```bash
# Install one of the 40+ bundled blueprints by name
factorly blueprint install slack

# Or paste a blueprint URL / GitHub repo / local file
factorly blueprint install github.com/factorly-dev/factorly-blueprints/gmail.yaml
factorly blueprint install ./my-blueprint.yaml
```

Browse the catalog in the UI at **Blueprints → Browse Catalog**.

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
factorly serve --port 3000
```

Endpoint at `http://localhost:3000/mcp`. Secure with `--http-token` or `FACTORLY_HTTP_TOKEN`. See [CLI Reference](cli-reference.md) for details. Use `--host 0.0.0.0` to bind to all interfaces (e.g. for containers).

> **Note:** If running inside Docker, use `host.docker.internal` instead of `localhost`. See [CLI Reference](cli-reference.md#http-server-authentication) for container setup.

## Next Steps

- [Config Reference](config-reference.md) — full YAML schema
- [Blueprints](blueprints.md) — 40+ pre-built service blueprints + bring-your-own from GitHub/URL/file
- [Vault](vault.md) — encrypted secret storage
- [OAuth](oauth.md) — authenticate with Google, GitHub, Microsoft

---

[← Back to Documentation](README.md)
