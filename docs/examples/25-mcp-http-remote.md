# MCP over HTTP

Serve Factorly's tools over HTTP so remote agents (or agents inside containers) can connect without stdio.

## Usage

```bash
# Start the HTTP server with token auth
factorly serve --port 3000 --http-token mytoken

# Or use a vault-stored token (recommended)
factorly vault set HTTP_TOKEN my-secret-token
factorly serve --port 3000 --http-token '{{vault:HTTP_TOKEN}}'
```

## Config

Connect Claude Code to the HTTP server by adding to `.mcp.json`:

```json
{
  "mcpServers": {
    "factorly": {
      "url": "http://localhost:3000/mcp",
      "headers": {
        "Authorization": "Bearer mytoken"
      }
    }
  }
}
```

### Docker setup

When Factorly runs inside a container, bind to all interfaces with `--host 0.0.0.0`:

```bash
# Inside the container
factorly serve --host 0.0.0.0 --port 3000 --http-token '{{vault:HTTP_TOKEN}}'
```

```json
{
  "mcpServers": {
    "factorly": {
      "url": "http://host.docker.internal:3000/mcp",
      "headers": {
        "Authorization": "Bearer mytoken"
      }
    }
  }
}
```

Or sync the config automatically:

```bash
factorly sync --http host.docker.internal:3000 --token '{{vault:HTTP_TOKEN}}'
```

## What happens

1. `factorly serve --port 3000` starts an HTTP server at `/mcp` using the MCP Streamable HTTP transport. By default it binds to `127.0.0.1` (localhost only) — use `--host 0.0.0.0` for containers.
2. `--http-token` enables Bearer token authentication. Every request must include `Authorization: Bearer <token>` or it receives a 401.
3. All configured tools are exposed over HTTP. Built-in tools (`factorly.shell`, `factorly.file.write`, etc.) are hidden in HTTP mode since local operations don't make sense on a remote server — only `factorly.fetch` remains available.
4. Use HTTP mode when: the agent runs in a different process/container, you need multiple agents sharing one server, or stdio isn't available. Use stdio mode (the default) for local, single-agent setups — it's simpler and has no auth overhead.

---

[← Back to Examples](README.md)
