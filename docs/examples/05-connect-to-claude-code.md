# Connect to Claude Code

Full onboarding: initialize Factorly, add tools, store credentials, and sync to Claude Code so your agent discovers tools automatically.

## Usage

```bash
# 1. Initialize the project
factorly init

# 2. Install the GitHub blueprint
factorly blueprint install github

# 3. Store your credentials in the vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxxxxxxxxxx

# 4. Sync to Claude Code (writes .mcp.json)
factorly sync
```

After syncing, Claude Code picks up the MCP configuration automatically. Your `.mcp.json` will look like this:

```json
{
  "mcpServers": {
    "factorly": {
      "command": "factorly",
      "args": ["serve"]
    }
  }
}
```

## Verify it works

```bash
# Check all tools are reachable
factorly tools status

# Test a call directly
factorly call github.list_repos --username octocat
```

Then in Claude Code:

```
> List my GitHub repos

I'll use the github.list_repos tool to look that up.

[Calling github.list_repos with username=octocat]

Found 30 repositories:
1. octocat/Hello-World
2. octocat/Spoon-Knife
...
```

## What happens

1. `factorly init` creates `.factorly/factorly.yaml` in your project.
2. `factorly blueprint install github` adds the bundled GitHub blueprint (list repos, issues, PRs, etc.).
3. `factorly vault set` encrypts your token with AES-256-GCM and stores it locally.
4. `factorly sync` writes `.mcp.json` so Claude Code knows to start Factorly as its MCP server.
5. When Claude Code launches, it starts `factorly serve`, discovers all your tools, and uses them — never seeing the underlying credentials.

---

[← Back to Examples](README.md)
