# Install a Blueprint

Use a bundled blueprint to get a fully configured Slack integration in 30 seconds.

## Usage

```bash
# Install the Slack blueprint from the bundled catalog.
# Prompts for your bot token and stores it in the encrypted vault.
factorly blueprint install slack

# (Optional) Set the token non-interactively
factorly vault set SLACK_BOT_TOKEN xoxb-xxxxxxxxxxxx-xxxxxxxxxxxx

# Call a tool the blueprint just added
factorly call slack.post_message --channel "#general" --text "Hello from Factorly"

# Verify everything's wired up
factorly tools status
```

Or, from the web UI: run `factorly ui`, click **Blueprints → Browse Catalog**, pick **Slack**, fill in your token, click **Install Blueprint**.

## What the blueprint looks like

Blueprints are self-contained YAML files. The Slack blueprint (excerpted) looks like:

```yaml
name: slack
display_name: Slack
version: 1.0.0
description: Team messaging, channels, notifications
category: business
auth_type: bearer
auth_guide: "Create a bot at https://api.slack.com/apps and copy the Bot User OAuth Token"

requires:
  vault_keys:
    - SLACK_BOT_TOKEN

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

After install, the file lives at `.factorly/blueprints/slack.yaml` — plain YAML, edit it by hand, commit it to git, or delete it to uninstall (`factorly blueprint uninstall slack` does the same thing).

## What happens

1. `factorly blueprint install slack` reads the bundled blueprint baked into the binary, validates it against your current config, and shows a preview.
2. It prompts for any vault keys the blueprint declares (here, `SLACK_BOT_TOKEN`) and stores them in the encrypted local vault.
3. The blueprint file is written to `.factorly/blueprints/slack.yaml`.
4. Tools like `slack.post_message` and `slack.list_channels` are immediately registered with the proxy.
5. Your agent discovers these tools via MCP and can use them — without ever seeing the bot token.

## Beyond the bundled catalog

Blueprints aren't limited to the bundled ones. Install from anywhere:

```bash
factorly blueprint install github.com/factorly-dev/factorly-blueprints/gmail.yaml
factorly blueprint install https://example.com/my-blueprint.yaml
factorly blueprint install ./local-blueprint.yaml
```

Or paste YAML directly into the UI's **Install Blueprint** modal.

See the [Blueprints reference](../blueprints.md) for the full format and how to author your own.

---

[← Back to Examples](README.md)
