# Lock Down for Production

A hardened config for deployed environments. Disable escape hatches, deny destructive operations, rate limit everything, and require confirmation for writes.

## Config

```yaml
# .factorly/factorly.yaml
disabled_commands:
  - vault       # no direct secret access
  - exec        # no ad-hoc command execution
  - wrap        # no wrapping arbitrary MCP servers

disable_builtins: true   # remove all factorly.* built-in tools

tools:
  github:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "{{vault:GITHUB_TOKEN}}"
    shadow:
      deny:
        - delete_repository
        - delete_branch
        - delete_release
      confirm:
        - merge_pull_request
        - create_release
        - update_branch_protection
      rate_limit: 200/hour
      log_params: [repo, branch, title]

  database:
    type: rest
    base_url: https://api.internal.acme.com/db
    method: POST
    path: /query
    auth:
      type: bearer
      token: "{{vault:DB_API_KEY}}"
    shadow:
      deny:
        - drop_table
        - truncate_table
      confirm: true
      rate_limit: 50/hour

  slack:
    type: mcp
    command: npx
    args: ["@anthropic/slack-mcp"]
    env:
      SLACK_BOT_TOKEN: "{{vault:SLACK_BOT_TOKEN}}"
    shadow:
      rate_limit: 30/min
```

## Usage

```bash
# Deploy and forget — governance is enforced at the config level
factorly serve
```

```bash
# These are all blocked
factorly vault list
# Error: command "vault" is disabled in .factorly/factorly.yaml

factorly exec -- curl https://example.com
# Error: command "exec" is disabled in .factorly/factorly.yaml

factorly call github.delete_repository --repo acme/api
# Error: tool "delete_repository" is denied by shadow policy on "github"
```

## What happens

1. `disabled_commands` removes CLI escape hatches — agents cannot access the vault, run arbitrary commands, or wrap new servers.
2. `disable_builtins: true` removes Factorly's built-in tools (echo, env, clipboard), leaving only the tools you explicitly define.
3. Shadow deny lists block destructive operations before they reach any service.
4. Shadow confirm rules require human approval for high-impact writes.
5. Rate limits cap total call volume per tool, preventing runaway loops.
6. `log_params` ensures key fields appear in the audit log for traceability.
7. Loop detection (always on) adds another layer — identical calls repeated 12+ times are auto-blocked.

---

[← Back to Examples](README.md)
