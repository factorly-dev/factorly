# Fetch and Transform

A code tool that calls the `factorly.fetch` builtin to hit a public API, then shapes the response before returning it.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  zen:
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
          return fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02"), res.Output), nil
      }
```

## Usage

```bash
factorly call zen
# [2026-05-16] It's not fully shipped until it's fast.
```

## What happens

1. `factorly call zen` invokes the code tool.
2. The script calls `factorly.Call("factorly.fetch", ...)`, which re-enters the proxy and runs the built-in fetch handler. The inner call gets its own audit log entry tagged `iface: code` so you can see "this fetch ran inside a script."
3. The script wraps the response with a timestamp and returns it as a string.
4. The proxy logs the outer call too, stamping `source_sha` with a SHA-256 of the script body.

## See also

- [Code Tools reference](../code-tools.md)
- [34: Hello Code Tool](34-hello-code-tool.md) — the bare-minimum version
- [36: Cross-tool Composition](36-cross-tool-composition.md) — calling multiple sibling tools and joining results

---

[← Back to Examples](README.md)
