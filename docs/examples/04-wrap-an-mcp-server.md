# Wrap an MCP Server

Proxy any existing MCP server through Factorly with zero configuration. Every call gets logged, output is compressed, and agent loops are detected automatically.

## Usage

```bash
# Wrap an MCP server — no config file needed
factorly wrap -- npx @modelcontextprotocol/server-everything

# After your agent makes some calls, check the logs
factorly logs -n 10

# View stats
factorly logs --stats
```

## What happens

1. Factorly spawns the MCP server (`npx @modelcontextprotocol/server-everything`) as a child process.
2. It discovers all tools the server exposes via the MCP protocol.
3. Factorly presents itself as an MCP server to your agent — same tools, same interface.
4. Every tool call from your agent passes through Factorly, which:
   - Logs the call (tool name, parameters, status, duration) to the audit log.
   - Compresses large JSON responses before returning them to the agent.
   - Detects repeated identical calls (agent loops) and rate-limits them.
5. Your agent connects to Factorly instead of the MCP server directly. No config file, no YAML, no changes to the original server.

You can also wrap a remote MCP server over HTTP:

```bash
factorly wrap --url http://localhost:3000/mcp
```

---

[← Back to Examples](README.md)
