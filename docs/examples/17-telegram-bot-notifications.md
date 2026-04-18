# Telegram Bot Notifications

Send messages to a Telegram chat from your agent using the Telegram Bot API.

## Setup

1. Message [@BotFather](https://t.me/BotFather) on Telegram and create a bot with `/newbot`.
2. Copy the bot token (looks like `7012345678:AAH...`).
3. Send a message to your bot, then fetch your chat ID:

```bash
curl -s "https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates" | jq '.result[0].message.chat.id'
```

## Config

```yaml
# .factorly/tools/telegram.yaml
tools:
  telegram.send_message:
    type: rest
    description: "Send a message to a Telegram chat"
    base_url: https://api.telegram.org
    method: POST
    path: /bot{{vault:TELEGRAM_BOT_TOKEN}}/sendMessage
    headers:
      Content-Type: application/json
    parameters:
      - name: chat_id
        in: body
        required: true
      - name: text
        in: body
        required: true
      - name: parse_mode
        in: body
    shadow:
      rate_limit: 30/min
```

## Usage

```bash
# Store your bot token and default chat ID
factorly vault set TELEGRAM_BOT_TOKEN "7012345678:AAHxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# Send a plain message
factorly call telegram.send_message \
  --chat_id 123456789 \
  --text "Deploy complete: payments-api v1.4.2 is live on staging."

# Send a formatted message
factorly call telegram.send_message \
  --chat_id 123456789 \
  --text "*Build failed*\n\`payments-api\` on \`main\`\nSee logs for details." \
  --parse_mode Markdown
```

## What happens

1. Factorly resolves `{{vault:TELEGRAM_BOT_TOKEN}}` in the path at call time — the agent never sees the token.
2. A `POST` is sent to `https://api.telegram.org/bot<token>/sendMessage` with `chat_id` and `text` in the JSON body.
3. Telegram delivers the message to the specified chat.
4. Rate limiting at 30/min prevents the agent from spamming the chat if it enters a notification loop.

---

[← Back to Examples](README.md)
