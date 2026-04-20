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

| Command | Strategy |
|---------|----------|
| `git status` | Strip hint lines ("use git add..."), max 30 lines |
| `git log` | Max 50 lines |
| `git diff` | Max 200 lines |
| `make` | Strip entering/leaving directory, short-circuit "Nothing to be done" |
| `npm install` | Strip warnings, short-circuit "up to date" / "added N packages" |
| `pnpm install` | Strip progress, short-circuit "Already up to date" |
| `go test` | Keep PASS/FAIL/ok lines, short-circuit all-pass |
| `cargo test` | Strip Compiling lines, short-circuit "test result: ok" |
| `cargo build` | Strip Compiling lines, short-circuit "Finished" |
| `pip install` | Strip "already satisfied", short-circuit "Successfully installed" |

Built-in filters are overridden by any user-defined `filter:` on the tool.

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
```

All regex patterns use Go's [regexp syntax](https://pkg.go.dev/regexp/syntax). Invalid patterns are logged as warnings and skipped — they won't break your config or block tool execution.

---

[← Back to Config Reference](config-reference.md) · [← Back to README](../README.md)
