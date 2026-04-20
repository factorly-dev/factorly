# Output Filters

Filters let you define command-specific output reduction rules per tool. They run after ANSI/whitespace cleanup but before JSON compaction and byte truncation, as part of the [output processing pipeline](config-reference.md#output-processing).

## Quick example

```yaml
tools:
  build.make:
    type: cli
    command: make
    filter:
      strip_lines:
        - "^make\\[\\d+\\]:"   # drop directory enter/leave noise
        - "^\\s*$"              # drop blank lines
      match_output:
        - pattern: "Nothing to be done"
          message: "ok (nothing to do)"
      max_lines: 50
```

With this filter, a noisy `make` output like:

```
make[1]: Entering directory '/project/src'
cc -o main main.c
make[1]: Leaving directory '/project/src'
```

Becomes:

```
cc -o main main.c
```

And a no-op build:

```
make: Nothing to be done for 'all'.
```

Becomes:

```
ok (nothing to do)
```

## Filter stages

Stages run in this order. Each is optional.

### match_output

Short-circuit the entire output. If a pattern matches the full output, replace it with a summary message. First match wins.

```yaml
filter:
  match_output:
    - pattern: "Build complete"
      message: "ok (build succeeded)"
      unless: "error|Error|FAIL"
```

The `unless` field prevents false positives — if the unless pattern also matches, the short-circuit is skipped and the output continues through the remaining stages. This is critical for commands that print "success" even when errors are present in the output.

### strip_lines

Drop lines matching any regex pattern:

```yaml
filter:
  strip_lines:
    - "^\\[INFO\\]"             # strip log noise
    - "^\\s*$"                  # strip blank lines
    - "^Requirement already"    # strip pip "already satisfied" lines
```

### keep_lines

Keep only lines matching a regex. Mutually exclusive with `strip_lines` — if both are set, `keep_lines` takes precedence.

```yaml
filter:
  keep_lines:
    - "^FAIL"
    - "^PASS"
    - "^ok\\s"
    - "^---"
```

### replace

Regex substitutions applied to each line, in order:

```yaml
filter:
  replace:
    - pattern: "/home/\\w+"
      replacement: "~"
    - pattern: "secret-[a-z0-9]+"
      replacement: "secret-***"
```

### head_lines / tail_lines

Line-based truncation. Keeps the first N and last M lines with `... X lines omitted ...` between them:

```yaml
filter:
  head_lines: 10
  tail_lines: 5
```

If the total line count is less than head + tail, no lines are omitted.

### max_lines

Absolute line count cap applied after all other filter stages:

```yaml
filter:
  max_lines: 50
```

## Built-in filters

Factorly ships with built-in filters for common commands. These apply automatically when `factorly.shell` or `factorly exec` runs a recognized command and no user-defined filter is configured.

### Build & test

| Command | Strategy |
|---------|----------|
| `make` | Strip entering/leaving directory, short-circuit "Nothing to be done" |
| `go test` | Keep PASS/FAIL/ok lines, short-circuit all-pass |
| `cargo test` | Strip Compiling lines, short-circuit "test result: ok" |
| `cargo build` | Strip Compiling lines, short-circuit "Finished" |
| `pytest` | Strip platform/plugin noise, short-circuit all-passed |
| `npm test` | Strip warnings, max 100 lines |
| `pnpm test` | Max 100 lines |

### Package managers

| Command | Strategy |
|---------|----------|
| `npm install` | Strip warnings, short-circuit "up to date" / "added N packages" |
| `pnpm install` | Strip progress, short-circuit "Already up to date" |
| `pip install` | Strip "already satisfied", short-circuit "Successfully installed" |
| `apt install` | Strip Hit/Get/Reading/Building noise, max 30 lines |
| `apt update` | Strip Hit/Get lines, short-circuit "All packages are up to date" |
| `brew install` | Strip download progress, short-circuit "already installed" |

### Git

| Command | Strategy |
|---------|----------|
| `git status` | Strip hint lines ("use git add..."), max 30 lines |
| `git log` | Max 50 lines |
| `git diff` | Max 200 lines |

### Shell & system

| Command | Strategy |
|---------|----------|
| `find` | Max 100 lines |
| `grep` | Max 100 lines |
| `rg` | Max 100 lines |
| `ps` | Max 50 lines |

### Containers & infrastructure

| Command | Strategy |
|---------|----------|
| `docker ps` | Max 50 lines |
| `docker logs` | Max 100 lines |
| `kubectl get` | Max 50 lines |
| `kubectl describe` | Max 100 lines |
| `kubectl logs` | Max 100 lines |
| `terraform plan` | Strip "Refreshing state", short-circuit "No changes" |
| `terraform apply` | Strip "Refreshing state", max 100 lines |

Built-in filters are overridden by any user-defined `filter:` on the tool.

### pipe

Pipe output through an external tool. The previous stages' output becomes the input, and the tool's response replaces it. Pipe supports all tool types — CLI, REST, and MCP — using the same config shape as any other tool definition.

If the tool fails or times out (default 5s), the output passes through unfiltered.

**CLI pipe** — output piped to stdin, stdout becomes the new output:

```yaml
filter:
  pipe:
    command: jq
    args: [".items[:10]"]
    timeout: 5s
```

```yaml
# Custom filter script
filter:
  pipe:
    command: ./scripts/filter-output.sh
    args: ["--format", "compact"]
    timeout: 10s
```

**REST pipe** — output POSTed to an API endpoint:

```yaml
filter:
  pipe:
    type: rest
    base_url: https://api.example.com
    method: POST
    path: /summarize
    auth:
      type: bearer
      token: "{{vault:API_KEY}}"
    timeout: 10s
```

**MCP pipe** — output passed to a tool on an MCP server:

```yaml
filter:
  pipe:
    type: mcp
    tool: summarize
    timeout: 10s
```

Pipe runs after all other filter stages, so the tool receives clean (ANSI-stripped, line-filtered) input.

## Full schema

```yaml
filter:
  match_output:
    - pattern: "regex"           # required
      message: "summary text"    # required
      unless: "regex"            # optional guard

  strip_lines:                   # list of regex patterns
    - "^noise"

  keep_lines:                    # list of regex patterns (overrides strip_lines)
    - "^important"

  replace:
    - pattern: "regex"           # required
      replacement: "text"        # required, supports $1 backreferences

  head_lines: 10                 # int, keep first N lines
  tail_lines: 5                  # int, keep last N lines
  max_lines: 50                  # int, absolute cap

  pipe:                          # external tool filter (cli, rest, or mcp)
    type: "cli"                  # "cli" (default), "rest", or "mcp"
    command: "executable"        # CLI: command to run
    args: ["arg1", "arg2"]       # CLI: arguments
    base_url: "https://..."      # REST: endpoint
    method: "POST"               # REST: HTTP method
    path: "/filter"              # REST: path
    tool: "tool_name"            # MCP: tool to call
    timeout: "5s"                # optional, default 5s
```

All regex patterns use Go's [regexp syntax](https://pkg.go.dev/regexp/syntax). Invalid patterns are logged as warnings and skipped. Pipe tools that fail or timeout fall back to unfiltered output.

---

[← Back to Config Reference](config-reference.md) · [← Back to README](../README.md)
