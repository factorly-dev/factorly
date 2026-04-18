# Install a Template

Use a pre-built template to get a fully configured Slack integration in 30 seconds.

## Usage

```bash
# List all available templates
factorly tools import templates

# Install the Slack template (interactive — prompts for your bot token)
factorly tools import templates slack

# Store the bot token in the vault
factorly vault set SLACK_BOT_TOKEN xoxb-xxxxxxxxxxxx-xxxxxxxxxxxx

# Call a tool from the template
factorly call slack.post_message --channel "#general" --text "Hello from Factorly"

# Verify the tool is working
factorly tools status
```

## Config

The template generates YAML like this in `.factorly/tools/slack.yaml`:

```yaml
tools:
  slack.post_message:
    type: rest
    description: "Post a message to a Slack channel"
    base_url: https://slack.com/api
    method: POST
    path: /chat.postMessage
    auth:
      type: bearer
      token: "{{vault:SLACK_BOT_TOKEN}}"
    parameters:
      - name: channel
        in: body
        required: true
      - name: text
        in: body
        required: true

  slack.list_channels:
    type: rest
    description: "List public channels in the workspace"
    base_url: https://slack.com/api
    method: GET
    path: /conversations.list
    auth:
      type: bearer
      token: "{{vault:SLACK_BOT_TOKEN}}"
    parameters:
      - name: limit
        in: query
```

## What happens

1. `factorly tools import templates slack` downloads the Slack template and writes tool definitions to `.factorly/tools/slack.yaml`.
2. It prompts you for a bot token and stores it in the vault.
3. Tools like `slack.post_message` and `slack.list_channels` are immediately available.
4. Your agent discovers these tools via MCP and can use them — without ever seeing the bot token.

---

[← Back to Examples](README.md)
