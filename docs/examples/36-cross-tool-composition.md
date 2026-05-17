# Cross-tool Composition

A code tool that calls multiple sibling tools, parses each result, and merges them into a single summary. This is where code tools really earn their keep — composition across heterogeneous tools is awkward as a declarative workflow but natural as ~30 lines of Go.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  user.profile:
    type: cli
    description: emit a fake user profile as JSON
    command: printf
    args: ['%s', '{"name":"Ada","email":"ada@example.com"}']

  user.prefs:
    type: cli
    description: emit fake preferences as JSON
    command: printf
    args: ['%s', '{"theme":"dark","timezone":"UTC"}']

  user.summary:
    type: code
    description: Build a one-line user summary from profile + prefs
    code: |
      package main

      import (
          "encoding/json"
          "errors"
          "fmt"

          "factorly"
      )

      func Run(params map[string]string) (any, error) {
          profileRes, err := factorly.Call("user.profile", nil)
          if err != nil {
              return nil, err
          }
          if profileRes.IsError() {
              return nil, errors.New(profileRes.Error)
          }
          prefsRes, err := factorly.Call("user.prefs", nil)
          if err != nil {
              return nil, err
          }
          if prefsRes.IsError() {
              return nil, errors.New(prefsRes.Error)
          }

          var profile struct {
              Name  string `json:"name"`
              Email string `json:"email"`
          }
          if err := json.Unmarshal([]byte(profileRes.Output), &profile); err != nil {
              return nil, fmt.Errorf("parsing profile: %w", err)
          }
          var prefs struct {
              Theme    string `json:"theme"`
              Timezone string `json:"timezone"`
          }
          if err := json.Unmarshal([]byte(prefsRes.Output), &prefs); err != nil {
              return nil, fmt.Errorf("parsing prefs: %w", err)
          }
          return fmt.Sprintf("%s <%s> — %s theme, %s",
              profile.Name, profile.Email, prefs.Theme, prefs.Timezone), nil
      }
```

## Usage

```bash
factorly call user.summary
# Ada <ada@example.com> — dark theme, UTC
```

## What happens

1. `factorly call user.summary` runs the code tool.
2. The script calls `user.profile` — proxy executes the CLI tool, returns JSON.
3. The script calls `user.prefs` — same path, different result.
4. Both responses are JSON-unmarshaled into typed Go structs.
5. The script returns a formatted string.

Three audit-log entries are written: one for `user.summary` (the outer code call, with `source_sha` stamped), and one each for `user.profile` and `user.prefs` (tagged `iface: code` to mark them as script-driven).

If either inner tool failed, the script propagates the error and the outer call exits with code 1. If the budget is exhausted (`max_calls` default 100), `factorly.Call` returns a Go error you can handle or surface.

## See also

- [Code Tools reference](../code-tools.md)
- [37: factorly.code Builtin](37-factorly-code-builtin.md) — the same engine, agent-supplied script

---

[← Back to Examples](README.md)
