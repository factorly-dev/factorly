# Deny Dangerous Operations

Block destructive tool calls before they reach the service. Shadow deny lists let you remove specific operations from an MCP server or REST tool without modifying the upstream config.

## Config

```yaml
# .factorly/tools/github.yaml
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

  database:
    type: rest
    base_url: https://db-admin.internal.acme.com
    method: POST
    path: /query
    auth:
      type: bearer
      token: "{{vault:DB_ADMIN_TOKEN}}"
    shadow:
      deny:
        - drop_table
        - truncate_table
        - delete_database
```

## Usage

```bash
# This works fine
factorly call github.list_repos --username octocat

# This is blocked immediately
factorly call github.delete_repository --repo octocat/Hello-World
# Error: tool "delete_repository" is denied by shadow policy on "github"
```

## What happens

1. The agent discovers all GitHub tools via MCP, including `delete_repository`.
2. The agent calls `github.delete_repository`.
3. Factorly checks the shadow deny list before making the call.
4. The call is rejected instantly — it never reaches GitHub.
5. The agent receives a clear error message and the denied call is logged in the audit trail.

---

[← Back to Examples](README.md)
