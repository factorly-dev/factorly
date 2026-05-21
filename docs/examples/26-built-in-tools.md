# Built-in Tools

Factorly ships five governed tools that work out of the box — no YAML needed. All are prefixed `factorly.` to avoid collisions. They execute **in-process** using Go's standard library (no subprocesses for read/write/fetch).

## Available tools

| Tool | Description | Implementation |
|------|-------------|----------------|
| `factorly.shell` | Run a shell command (confirm required, destructive patterns blocked) | `exec.Command` with 30s timeout |
| `factorly.file.read` | Read a local file (sensitive paths blocked) | `os.ReadFile`, project-scoped |
| `factorly.file.write` | Write a local file (confirm required, system paths blocked) | `os.WriteFile`, project-scoped |
| `factorly.fetch` | HTTP GET a URL (cloud metadata and private networks blocked) | `net/http`, 1MB limit |
| `factorly.clipboard` | Copy text to clipboard (confirm required) | Platform-aware (pbcopy/xclip/xsel) |

## Usage

```bash
# List built-in tools alongside your configured tools
factorly tools
```

```
  factorly.shell       built-in  Run a shell command
  factorly.file.read   built-in  Read a local file
  factorly.file.write  built-in  Write a local file
  factorly.fetch       built-in  HTTP GET a URL
  factorly.clipboard   built-in  Copy text to clipboard
  github.repos         rest      GET /users/{username}/repos
```

## Project scoping

File operations (`read_file`, `write_file`) are **scoped to the project directory** (the directory containing your `factorly.yaml`). Any attempt to read or write outside this directory is rejected:

```
path "/etc/passwd" is outside project directory "/home/user/myproject"
```

Relative paths are resolved against the project root. Path traversal (`../../../etc/passwd`) is blocked.

## Safety guards

Built-in tools block dangerous operations by default:

- **Shell**: blocks `rm -rf /`, `curl | sh`, `DROP TABLE`, `shutdown`, fork bombs. Uses `sh -c` on Unix, `cmd /C` on Windows.
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

  factorly.file.read:
    shadow:
      allow_paths: [".env.example", ".env.template"]

  factorly.fetch:
    shadow:
      allow_urls: ["http://localhost:8080"]
```

Disable specific built-ins or all of them:

```yaml
# Disable specific built-in tools
disabled_builtins:
  - factorly.shell
  - factorly.clipboard

# Or disable all built-ins
disable_builtins: true
```

## What happens

1. Built-in tools are registered automatically when Factorly starts. They appear in `factorly tools` alongside your configured tools.
2. Each built-in executes in-process (no subprocess), except `clipboard` which uses the platform clipboard command.
3. File operations are scoped to the project directory. Shell commands run from the working directory.
4. Each built-in has default oversight rules (confirm prompts, blocked patterns). These apply without any config.
5. You can loosen restrictions using `allow_patterns`, `allow_paths`, or `allow_urls` in a `shadow` block — but only for specific values, not blanket overrides.
6. In HTTP mode (`factorly serve --port`), local tools (`shell`, `read_file`, `write_file`, `clipboard`) are hidden. Only `factorly.fetch` is available since local filesystem operations don't apply to remote servers.
7. In the UI (`factorly ui`), built-in tools appear with a "Built-in" badge and locked configuration — only oversight settings are editable.

---

[← Back to Examples](README.md)
