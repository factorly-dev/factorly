# factorly.code Builtin

`factorly.code` is a built-in tool that accepts a Go script *as a parameter*. The script body is supplied at call time — by you, or by an agent through MCP. Same yaegi sandbox, same `factorly.Call` SDK, but the source is dynamic.

The builtin is registered automatically when factorly runs in stdio mode (`factorly call ...`, `factorly serve` without `--http`, the UI's embedded MCP). HTTP serve mode excludes it — same gating as `factorly.shell`.

## Usage

```bash
factorly call factorly.code \
  --code 'package main
import "fmt"
func Run(p map[string]string) (any, error) {
    return fmt.Sprintf("hello %s (%s)", p["name"], p["greeting"]), nil
}' \
  --params '{"name":"alice","greeting":"hi"}'
# hello alice (hi)
```

Two parameters:

- **`code`** (required) — the full Go source. Must declare `func Run(params map[string]string) (any, error)`.
- **`params`** (optional, JSON object) — gets unpacked into the `map[string]string` the script's `Run` receives.

## Calling other tools

The script's reach is identical to `type: code` tools — anything in your tool registry is callable:

```bash
factorly call factorly.code --code '
package main
import (
    "errors"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    res, err := factorly.Call("factorly.fetch", map[string]string{
        "url": "https://api.github.com/zen",
    })
    if err != nil {
        return nil, err
    }
    if res.IsError() {
        return nil, errors.New(res.Error)
    }
    return res.Output, nil
}' --params '{}'
```

## Gating it

By default the agent can invoke `factorly.code` without prompting. To require human approval per invocation, override the builtin's shadow config:

```yaml
# .factorly/factorly.yaml
tools:
  factorly.code:
    type: builtin
    shadow:
      confirm: true       # human approval on every invocation
      max_calls: 20       # cap inner factorly.Call invocations per script run
      rate_limit: 5/min   # at most 5 script invocations per minute
```

The user-side shadow merges over the builtin's defaults — every other field (type, parameters, description) stays builtin-controlled.

## What happens

1. The agent (or you) calls `factorly.code` with `code` and `params` arguments.
2. The builtin handler computes `source_sha = SHA-256(code)` and stamps it on the audit log entry.
3. yaegi compiles the source fresh (no cache in V2), invokes `Run(params)`.
4. The script's return value flows back through the proxy — string passes through, anything else becomes JSON.
5. Inner `factorly.Call` invocations get their own audit log entries tagged `iface: code`.

## When to use this vs `type: code`

| Use `type: code` (V1) when... | Use `factorly.code` (V2) when... |
|-------------------------------|----------------------------------|
| The script is stable and reusable | The agent composes the script per task |
| You want a named, callable tool | You want a generic "run this Go" capability |
| Power-user authoring in YAML | LLM-authored at runtime via MCP |

Both share the engine, the SDK, the budgets, and the audit trail.

## See also

- [Code Tools reference](../code-tools.md)
- [34: Hello Code Tool](34-hello-code-tool.md) — the `type: code` equivalent

---

[← Back to Examples](README.md)
