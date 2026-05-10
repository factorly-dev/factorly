# Require Confirmation

Require human approval before high-impact operations execute. Shadow confirm works on any tool type and supports both blanket confirmation and per-action granularity.

## Config

```yaml
# .factorly/tools/guarded.yaml
tools:
  stripe:
    type: mcp
    command: npx
    args: ["@anthropic/stripe-mcp"]
    env:
      STRIPE_SECRET_KEY: "{{vault:STRIPE_KEY}}"
    shadow:
      confirm:
        - create_charge
        - create_refund

  github:
    type: mcp
    command: npx
    args: ["@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "{{vault:GITHUB_TOKEN}}"
    shadow:
      confirm:
        - merge_pull_request
        - create_release

  email:
    type: rest
    base_url: https://api.sendgrid.com/v3
    method: POST
    path: /mail/send
    auth:
      type: bearer
      token: "{{vault:SENDGRID_KEY}}"
    shadow:
      confirm: true   # every call requires approval
```

## Usage

```bash
# Agent tries to merge a PR — you get a prompt
factorly call github.merge_pull_request --repo acme/api --pull_number 42
# ⚠ Tool "merge_pull_request" requires confirmation. Proceed? (y/n)

# Agent tries to send an email — you get a prompt (confirm: true catches all)
factorly call email --to "team@acme.com" --subject "Deploy complete"
# ⚠ Tool "email" requires confirmation. Proceed? (y/n)
```

## What happens

1. The agent calls a tool that matches a confirm rule.
2. **CLI mode** (`factorly call`): Factorly prompts on stderr — `⚠ Tool "x" requires confirmation. Proceed? (y/n)`. The call only executes if you type `y`.
3. **MCP mode** (`factorly serve`): Factorly uses MCP elicitation. The AI client (Claude Code, Cursor) displays the confirmation prompt in the chat UI.
4. **UI mode** (`factorly ui`): Confirmation prompts appear as a modal in the browser. If an MCP agent is also connected, both the browser and the agent see the prompt — **first response wins**.
5. If no response is received within **60 seconds**, the call is automatically **denied**.
6. If denied, the agent receives an error and the rejection is logged.
7. `confirm: true` guards every call on that tool. `confirm: [action1, action2]` only guards the listed sub-tools.

### Verbose logging

With `--verbose`, Factorly logs confirmation routing:

```
[confirm] factorly.shell: waiting for approval (browser, elicitation)
[confirm] factorly.shell: approved via browser
```

---

[← Back to Examples](README.md)
