# Code Tools

A **code tool** is a Factorly tool whose body is a Go script. The script runs inside a sandboxed [yaegi](https://github.com/traefik/yaegi) interpreter and can call any other registered tool through `factorly.Call`. Code tools shine when you need branching, loops, data shaping, or composition across multiple tools — things that get awkward as declarative workflows.

There are two flavors:

1. **`type: code` tool** — power-user authoring. You write the Go body in YAML; Factorly compiles it once at registration. Lives at `.factorly/<file>.yaml` like any other tool.
2. **`factorly.code` builtin** — agent-authored. The script is a parameter; an LLM composes it at call time. Same sandbox, same SDK.

Both share the same engine, the same `factorly.Call` SDK, and the same audit trail.

## The contract

Every script declares one function:

```go
func Run(params map[string]string) (any, error)
```

- **`params`** is the tool's call-time arguments. Always non-nil.
- **Return value** becomes the tool's output:
  - `string` → passes through verbatim
  - anything else → `json.Marshal`'d
  - `nil` → empty output
- **Return error** → tool reports `ExitCode: 1` with the error message
- **Panic** → recovered, reported as `ExitCode: 2`

The script must start with a `package <name>` declaration (the name itself doesn't matter — `package main` works).

## What scripts can import

A minimal stdlib subset, plus the `factorly` host package:

```go
import (
    "fmt"
    "strings"
    "strconv"
    "time"
    "encoding/json"
    "errors"

    "factorly"  // host-provided SDK
)
```

Everything else (notably `os`, `net`, `reflect`, `unsafe`) is denied at compile time. Scripts touch the outside world only through `factorly.Call`.

## The `factorly` SDK

```go
package factorly

// Call another registered Factorly tool. Returns the same Result
// shape the proxy uses internally; check IsError() to detect
// tool-level failures and check err for infrastructure failures.
func Call(name string, params map[string]string) (*Result, error)

// Snapshot of currently-registered, non-hidden tools.
// Stable for the duration of one Run.
func ListTools() []ToolInfo

type Result struct {
    Output   string
    Error    string
    ExitCode int
    Duration time.Duration
}
func (r *Result) IsError() bool

type ToolInfo struct {
    Name        string
    Description string
    Parameters  []ParamInfo
}

type ParamInfo struct {
    Name        string
    Type        string  // "string", "boolean", etc. — empty defaults to "string"
    Required    bool
    Description string
    Default     string
}
```

## Authoring a `type: code` tool

```yaml
# .factorly/my-tools.yaml
tools:
  greet:
    type: code
    description: Greet someone by name
    parameters:
      - name: name
        required: true
        default: world
    code: |
      package main

      func Run(params map[string]string) (any, error) {
          return "hello " + params["name"], nil
      }
```

Call it:

```bash
factorly call greet --name alice
# hello alice
```

If the script fails to compile, `factorly tools` lists the tool but the error is logged at startup and any call returns `script failed to compile: <yaegi message>`. Fix the source, save, the next invocation re-runs.

## Calling other tools

A code tool that fetches JSON and reshapes it:

```yaml
tools:
  github.zen:
    type: code
    description: Fetch the GitHub Zen koan and tag it with a timestamp
    code: |
      package main

      import (
          "errors"
          "fmt"
          "time"

          "factorly"
      )

      func Run(params map[string]string) (any, error) {
          res, err := factorly.Call("factorly.fetch", map[string]string{
              "url": "https://api.github.com/zen",
          })
          if err != nil {
              return nil, err
          }
          if res.IsError() {
              return nil, errors.New(res.Error)
          }
          return fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), res.Output), nil
      }
```

Every inner `factorly.Call` re-enters the proxy and goes through its full shadow/vault/audit machinery. The script can only reach what's already exposed as a tool.

## The `factorly.code` builtin (agent-authored scripts)

The builtin's parameters *are* the script:

```bash
factorly call factorly.code \
  --code 'package main
func Run(p map[string]string) (any, error) { return "hi " + p["who"], nil }' \
  --params '{"who":"world"}'
# hi world
```

Same engine, same SDK. The `params` argument is a JSON object; its keys/values become the `params map[string]string` the script's `Run` receives.

The builtin is registered in `stdio` mode only (same gating as `factorly.shell`). HTTP-mode MCP servers don't expose it.

## Oversight

Code tools are tool calls, full stop. The shadow layer treats them like any other tool:

```yaml
factorly.code:
  type: builtin
  shadow:
    confirm: true        # human approval on every invocation
    max_calls: 50        # cap inner factorly.Call invocations per script run
    rate_limit: 10/min   # standard rate limiting applies
```

And inner `factorly.Call` invocations are subject to *their own* tool's shadow rules — so a script trying to call a `confirm: true` tool still triggers the confirm prompt for each call.

## Budgets

| Setting | Default | Configured via |
|---------|---------|----------------|
| Wall-clock timeout | per-tool `timeout:` field (or unlimited) | `tools.X.timeout: "30s"` |
| Inner-call cap | 100 | `tools.X.shadow.max_calls: 200` |
| Output size cap | per-tool `max_output` (or unlimited) | `tools.X.max_output: 50000` |

Exceeding `max_calls` returns a Go error from `factorly.Call`, which the script can handle or propagate.

## Audit trail

Every call to a code tool stamps a **`source_sha`** field on the audit log entry — a SHA-256 of the script body that ran. For `type: code` tools the hash is over the stashed source; for `factorly.code` it's over the agent-supplied `code` param. Inner calls inherit the parent's agent ID and are tagged with `iface: code` so the audit log can distinguish "script-driven" from "directly invoked" calls.

View the trail:

```bash
factorly logs --tail 5
```

## Patterns

### Strings vs structured output

```go
// String returns pass through verbatim:
return "ok", nil
// Output: ok

// Structured returns are JSON-marshaled:
return map[string]int{"count": 42}, nil
// Output: {"count":42}

// Slices, structs, anything Marshalable works:
return []string{"a", "b", "c"}, nil
// Output: ["a","b","c"]
```

If you want JSON output, return the struct or map directly — don't `json.Marshal` it yourself, the provider does that for you. (You will see double-encoding if you do.)

### Reading params with defaults

All params are strings on the wire. Use `strconv` to coerce, and fall back to the declared default if you want:

```go
limit := 10
if v := params["limit"]; v != "" {
    if n, err := strconv.Atoi(v); err == nil {
        limit = n
    }
}
```

If the tool's YAML declares `default: "10"` on the param, the proxy populates `params["limit"]` before the script runs, so the conditional above is enough.

### Discovery before composition

A script that picks a tool based on what's registered:

```go
for _, t := range factorly.ListTools() {
    if strings.HasPrefix(t.Name, "gmail.") {
        // ...
    }
}
```

Hidden tools (`hidden: true`) are excluded from `ListTools` — they're still callable by name if the script hardcodes it, but the agent-facing discovery surface treats them as private.

### Looping over a tool result

A script that paginates through a list:

```go
func Run(params map[string]string) (any, error) {
    var all []string
    cursor := ""
    for {
        params := map[string]string{}
        if cursor != "" {
            params["cursor"] = cursor
        }
        res, err := factorly.Call("api.list", params)
        if err != nil {
            return nil, err
        }
        // ... parse, append to all, update cursor, break when empty
        if cursor == "" {
            break
        }
    }
    return all, nil
}
```

Loop detection on the *outer* code tool fires per the usual fingerprinting; loop detection on inner `factorly.Call` invocations fires per *those* fingerprints. The `max_calls` budget puts a hard ceiling on inner volume regardless of fingerprints.

## Examples

See the example pages for runnable scripts:

- [34: Hello Code Tool](examples/34-hello-code-tool.md) — minimum viable script with a parameter
- [35: Fetch and Transform](examples/35-fetch-and-transform.md) — call `factorly.fetch`, parse JSON, return a field
- [36: Cross-tool Composition](examples/36-cross-tool-composition.md) — script that calls multiple sibling tools and joins results
- [37: factorly.code Builtin](examples/37-factorly-code-builtin.md) — the agent-authored entry point

## Out of scope (V3 candidates)

- **`code_file:`** — referencing external `.go` files. Today the body must be inline.
- **Confirm-on-source-change** for `factorly.code` — approve once per SHA, replay within a window. Today the user-side path is `shadow: confirm: true` which prompts every call.
- **JS engine** — QuickJS-backed `type: codejs` and `factorly.codejs` were explored; deferred until the Go path gets real-world use.

---

[← Back to Documentation](README.md)
