# Pipe Filters

Pipe tool output through external commands for advanced filtering that goes beyond regex. The pipe tool receives the output on stdin (for CLI) or as input (for REST/MCP), and its response replaces the original output.

## JSON transformation with jq

Extract specific fields from a verbose API response:

```yaml
# .factorly/factorly.yaml
tools:
  github.issues:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /repos/{{owner}}/{{repo}}/issues
    auth:
      type: bearer
      token: "{{vault:GITHUB_TOKEN}}"
    filter:
      pipe:
        command: jq
        args: ["[.[:10] | .[] | {number, title, state}]"]
        timeout: 5s
```

A 50KB issues response becomes a compact list of the first 10 issues with just number, title, and state.

## Filter with grep

Keep only error lines from verbose output:

```yaml
# .factorly/factorly.yaml
tools:
  build.run:
    type: cli
    command: cargo
    args: ["build", "--release"]
    filter:
      strip_lines:
        - "^\\s+Compiling "
        - "^\\s+Downloading "
      pipe:
        command: grep
        args: ["-E", "error|warning"]
        timeout: 5s
```

Strip and pipe work together — first strip removes compiling/downloading noise, then grep keeps only error and warning lines. If grep finds nothing (exit code 1), output passes through unfiltered.

## Custom filter script

Use a project-specific script for domain-specific filtering:

```yaml
# .factorly/factorly.yaml
tools:
  test.integration:
    type: cli
    command: pytest
    args: ["-v", "tests/integration/"]
    filter:
      pipe:
        command: ./scripts/test-summary.sh
        timeout: 10s
```

The script receives test output on stdin and can do anything — parse XML, extract metrics, format a summary.

## Pipe with timeout

Long-running pipe commands are killed after the timeout (default 5s). Output falls back to unfiltered:

```yaml
# .factorly/factorly.yaml
tools:
  data.export:
    type: cli
    command: pg_dump
    args: ["--schema-only", "{{database}}"]
    filter:
      max_lines: 200
      pipe:
        command: sed
        args: ["/^--/d"]
        timeout: 3s
```

## What happens

1. The tool runs and produces output.
2. ANSI codes are stripped and whitespace is normalized (always-on).
3. Filter stages run in order: match_output → strip/keep → replace → head/tail → max_lines.
4. The pipe command receives the filtered output on stdin.
5. The pipe command's stdout replaces the output.
6. If the pipe fails or times out, the pre-pipe output is used instead (fail-safe).
7. JSON compaction and log dedup run after the pipe (if compress hints are set).
8. Byte truncation applies last (if max_output is set).

---

[← Back to Examples](README.md)
