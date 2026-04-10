```
░█▀▀░█▀█░█▀▀░▀█▀░█▀█░█▀▄░█░░█░█
░█▀▀░█▀█░█░░░░█░░█░█░█▀▄░█░░░█░
░▀░░░▀░▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀▀▀░▀░
```

<center>

# Factorly

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen?logo=github)](https://github.com/factorly-dev/factorly-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/Release-v0.1.2-blue?logo=github)](https://github.com/factorly-dev/factorly-cli/releases)
[![MCP](https://img.shields.io/badge/MCP-compatible-8A2BE2)](https://modelcontextprotocol.io)
[![Docs](https://img.shields.io/badge/Docs-docs%2F-informational)](docs/)

One endpoint. All your tools. Credentials stay out of your agent's hands.  
  
Factorly wraps REST APIs, CLIs, and MCP servers into a single, governed interface. Configure your tools once, connect your agent once, and every call is authenticated, proxied, and logged. Your agent never touches a secret.  

</center>

## Install

```bash
npm install -g @factorly/cli
```

Or run directly:

```bash
npx @factorly/cli version
```

## Quick Start

```bash
# Initialize a project
factorly init

# Add a tool from a curl command
echo 'curl -H "Authorization: Bearer tok" https://api.example.com/data' | factorly tools record

# Connect to Claude Code / Cursor / Codex
factorly sync

# Start as an MCP server
factorly serve
```

## What It Does

- Wraps **REST APIs**, **CLI commands**, and **MCP servers** into one endpoint
- Secrets stay in Factorly's encrypted vault — the agent never sees them
- Every tool call is logged with timestamp, parameters, and response
- OAuth 2.0 with auto-refresh for Google, GitHub, Microsoft, Slack

## Documentation

Full documentation at [github.com/factorly-dev/factorly-cli](https://github.com/factorly-dev/factorly-cli)

## Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | x64 |
| macOS | x64, arm64 |
| Windows | x64 |

The npm package downloads the pre-built Go binary for your platform during install.

## License

MIT
