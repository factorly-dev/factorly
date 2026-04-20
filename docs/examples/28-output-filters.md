# Output Filters

Reduce tool output noise with command-specific filters. Strip verbose lines, short-circuit success output to single-line summaries, and pipe output through external tools.

## Strip noisy lines

Remove verbose build output that wastes tokens:

```yaml
# .factorly/factorly.yaml
tools:
  build.gradle:
    type: cli
    command: gradle
    args: ["{{task}}"]
    filter:
      strip_lines:
        - "^Download"
        - "^Generating"
        - "^\\s*$"
      max_lines: 50
```

**Before** (80 lines of download/generate noise + 3 lines of results):
```
Downloading https://repo.maven.org/...
Downloading https://repo.maven.org/...
Generating build scripts...

BUILD SUCCESSFUL in 12s
3 actionable tasks: 3 executed
```

**After** (2 lines):
```
BUILD SUCCESSFUL in 12s
3 actionable tasks: 3 executed
```

## Short-circuit success output

Replace verbose success output with a one-line summary. The `unless` guard prevents false positives when errors are hidden in the output:

```yaml
# .factorly/factorly.yaml
tools:
  ci.test:
    type: cli
    command: npm
    args: ["test"]
    filter:
      match_output:
        - pattern: "Tests:.*passed"
          message: "ok (all tests passed)"
          unless: "fail|FAIL|Error"
      strip_lines:
        - "^\\s*$"
        - "^PASS "
      max_lines: 100
```

**On success** — 200 lines of test output becomes:
```
ok (all tests passed)
```

**On failure** — the `unless` guard fires, full output shown (with blank lines and PASS lines stripped).

## Keep only important lines

Inverse of strip — keep only lines matching patterns, drop everything else:

```yaml
# .factorly/factorly.yaml
tools:
  test.go:
    type: cli
    command: go
    args: ["test", "-v", "./..."]
    filter:
      keep_lines:
        - "^--- FAIL"
        - "^FAIL"
        - "^ok\\s"
        - "^PASS$"
```

A verbose Go test run with hundreds of `=== RUN` and `--- PASS` lines becomes just the summary.

## Regex replace

Transform output lines — redact paths, collapse verbose patterns:

```yaml
# .factorly/factorly.yaml
tools:
  deploy.status:
    type: cli
    command: kubectl
    args: ["get", "pods", "-o", "wide"]
    filter:
      replace:
        - pattern: "\\d+\\.\\d+\\.\\d+\\.\\d+"
          replacement: "<ip>"
        - pattern: "/home/\\w+"
          replacement: "~"
      max_lines: 50
```

## Head and tail

Keep the first and last N lines with an omission marker between:

```yaml
# .factorly/factorly.yaml
tools:
  logs.app:
    type: cli
    command: tail
    args: ["-1000", "/var/log/app.log"]
    filter:
      head_lines: 5
      tail_lines: 10
```

**Output:**
```
2026-04-20 09:00:01 Starting application...
2026-04-20 09:00:02 Loading config...
2026-04-20 09:00:02 Connecting to database...
2026-04-20 09:00:03 Server listening on :8080
2026-04-20 09:00:04 Ready
... 985 lines omitted ...
2026-04-20 10:42:51 Request processed in 12ms
2026-04-20 10:42:52 Request processed in 8ms
...
```

## Pipe through a CLI tool

Pass output through any external command. Previous filter stages run first, so the pipe gets clean input:

```yaml
# .factorly/factorly.yaml
tools:
  api.search:
    type: rest
    base_url: https://api.example.com
    method: GET
    path: /search
    auth:
      type: bearer
      token: "{{vault:API_KEY}}"
    filter:
      pipe:
        command: jq
        args: [".results[:5] | .[].title"]
        timeout: 5s
```

A large JSON search response is filtered down to just the first 5 titles by `jq`.

## Combine stages

Stages run in order: match_output → strip/keep → replace → head/tail → max_lines → pipe. Combine them for precise control:

```yaml
# .factorly/factorly.yaml
tools:
  build.make:
    type: cli
    command: make
    args: ["{{target}}"]
    filter:
      match_output:
        - pattern: "Nothing to be done"
          message: "ok (nothing to do)"
      strip_lines:
        - "^make\\[\\d+\\]:"
        - "^\\s*$"
      replace:
        - pattern: "warning: "
          replacement: "⚠ "
      max_lines: 50
```

## What happens

1. `match_output` checks the entire output — if "Nothing to be done" matches, returns `"ok (nothing to do)"` immediately and skips all other stages.
2. `strip_lines` removes `make[1]: Entering/Leaving directory` noise and blank lines.
3. `replace` transforms `warning:` prefixes for readability.
4. `max_lines` caps at 50 lines if the build output is still long.
5. `pipe` (if configured) passes the result through an external tool.
6. Built-in filters apply automatically for common commands (git, make, npm, go test, cargo, pip, and more) when no user-defined filter is set.

---

[← Back to Examples](README.md)
