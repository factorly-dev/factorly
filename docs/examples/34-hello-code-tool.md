# Hello Code Tool

The minimum viable `type: code` tool — a Go script with one parameter, no inner tool calls.

## Config

```yaml
# .factorly/factorly.yaml
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

## Usage

```bash
factorly call greet --name alice
# hello alice

factorly call greet
# hello world          (uses the declared default)
```

## What happens

1. `factorly call greet --name alice` finds the tool in your config.
2. The proxy validates `name` against the declared parameters (required, default applied if missing) and passes a `map[string]string` to the script.
3. yaegi loads the script (compiled once at registration), invokes `Run(params)`.
4. The returned string becomes the tool's output and is logged in the audit trail.

## See also

- [Code Tools reference](../code-tools.md) — the full SDK and contract
- [35: Fetch and Transform](35-fetch-and-transform.md) — calling another tool from inside a script

---

[← Back to Examples](README.md)
