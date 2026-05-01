# Built-in Tools

Factorly ships five governed tools that work out of the box — no YAML needed. All are prefixed `factorly.` to avoid collisions.

## Available tools

| Tool | Description |
|------|-------------|
| `factorly.shell` | Run a shell command (confirm required, destructive patterns blocked) |
| `factorly.read_file` | Read a local file (sensitive paths blocked) |
| `factorly.write_file` | Write a local file (confirm required, system paths blocked) |
| `factorly.fetch` | HTTP GET a URL (cloud metadata and private networks blocked) |
| `factorly.clipboard` | Copy text to clipboard (confirm required) |

## Usage

```bash
# List built-in tools alongside your configured tools
factorly tools
```

```
  factorly.shell       built-in  Run a shell command
  factorly.read_file   built-in  Read a local file
  factorly.write_file  built-in  Write a local file
  factorly.fetch       built-in  HTTP GET a URL
  factorly.clipboard   built-in  Copy text to clipboard
  github.repos         rest      GET /users/{username}/repos
```

## Safety guards

Built-in tools block dangerous operations by default:

- **Shell**: blocks `rm -rf /`, `curl | sh`, `DROP TABLE`, `shutdown`, fork bombs
- **Read/Write**: blocks `.env`, `.ssh/id_*`, `*.pem`, `credentials.json`, `/etc/shadow`, system directories
- **Fetch**: blocks cloud metadata (`169.254.169.254`), localhost, private networks, `file://` protocol

## Config

Override default denials for specific cases:

```yaml
# .factorly/factorly.yaml
tools:
  factorly.shell:
    shadow:
      allow_patterns: ["rm -rf ./build"]

  factorly.read_file:
    shadow:
      allow_paths: [".env.example", ".env.template"]

  factorly.fetch:
    shadow:
      allow_urls: ["http://localhost:8080"]
```

Disable specific built-ins or all of them:

```bash
# Disable specific tools via env var
export FACTORLY_DISABLED_TOOLS=factorly.shell,factorly.write_file
```

```yaml
# Or disable all built-ins in config
disable_builtins: true
```

## What happens

1. Built-in tools are registered automatically when Factorly starts. They appear in `factorly tools` alongside your configured tools.
2. Each built-in has default oversight rules (confirm prompts, blocked patterns). These apply without any config.
3. You can loosen restrictions using `allow_patterns`, `allow_paths`, or `allow_urls` in a `shadow` block — but only for specific values, not blanket overrides.
4. In HTTP mode (`factorly serve --http`), local tools (`shell`, `read_file`, `write_file`, `clipboard`) are hidden. Only `factorly.fetch` is available since local filesystem operations don't apply to remote servers.

---

[← Back to Examples](README.md)
