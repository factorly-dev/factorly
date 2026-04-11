package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Telegram returns the template for Telegram Bot API.
func Telegram() *Template {
	return &Template{
		Name:        "telegram",
		DisplayName: "Telegram",
		Description: "Bot messaging, channels, and notifications via phone",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a bot with @BotFather on Telegram — it will give you the token",
		VaultKey:    "TELEGRAM_BOT_TOKEN",
		BaseURL:     "https://api.telegram.org",
		Tools: []ToolDef{
			{
				Name:        "send_message",
				Description: "Send a text message to a chat",
				Method:      "POST",
				Path:        "/bot{{token}}/sendMessage",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token (auto-filled from vault)"},
					{Name: "chat_id", In: "body", Required: true, Description: "Chat ID or @channel_username"},
					{Name: "text", In: "body", Required: true, Description: "Message text (supports Markdown)"},
					{Name: "parse_mode", In: "body", Description: "Formatting: Markdown, MarkdownV2, or HTML"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "get_updates",
				Description: "Get incoming updates (messages, commands, etc.)",
				Method:      "POST",
				Path:        "/bot{{token}}/getUpdates",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token"},
					{Name: "offset", In: "body", Description: "Update offset for pagination"},
					{Name: "limit", In: "body", Description: "Max number of updates (1-100)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "send_document",
				Description: "Send a file to a chat",
				Method:      "POST",
				Path:        "/bot{{token}}/sendDocument",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token"},
					{Name: "chat_id", In: "body", Required: true, Description: "Chat ID or @channel_username"},
					{Name: "document", In: "body", Required: true, Description: "File URL or file_id"},
					{Name: "caption", In: "body", Description: "Document caption"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_chat",
				Description: "Get information about a chat",
				Method:      "POST",
				Path:        "/bot{{token}}/getChat",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token"},
					{Name: "chat_id", In: "body", Required: true, Description: "Chat ID or @channel_username"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "send_photo",
				Description: "Send a photo to a chat",
				Method:      "POST",
				Path:        "/bot{{token}}/sendPhoto",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token"},
					{Name: "chat_id", In: "body", Required: true, Description: "Chat ID or @channel_username"},
					{Name: "photo", In: "body", Required: true, Description: "Photo URL or file_id"},
					{Name: "caption", In: "body", Description: "Photo caption"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_me",
				Description: "Get information about the bot",
				Method:      "POST",
				Path:        "/bot{{token}}/getMe",
				Parameters: []config.ParamConfig{
					{Name: "token", In: "path", Required: true, Description: "Bot token"},
				},
				ActionType: "read",
				Essential:  true,
			},
		},
	}
}
