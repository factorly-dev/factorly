package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Slack returns the template for Slack team messaging.
func Slack() *Template {
	return &Template{
		Name:        "slack",
		DisplayName: "Slack",
		Description: "Team messaging, channels, and notifications",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a bot token at https://api.slack.com/apps (Bot User OAuth Token)",
		VaultKey:    "SLACK_BOT_TOKEN",
		BaseURL:     "https://slack.com/api",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "post_message",
				Description: "Post a message to a channel",
				Method:      "POST",
				Path:        "/chat.postMessage",
				Parameters: []config.ParamConfig{
					{Name: "channel", In: "body", Required: true, Description: "Channel ID or name"},
					{Name: "text", In: "body", Required: true, Description: "Message text"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_channels",
				Description: "List channels in the workspace",
				Method:      "GET",
				Path:        "/conversations.list",
				Parameters: []config.ParamConfig{
					{Name: "types", In: "query", Description: "Channel types (public_channel,private_channel)"},
					{Name: "limit", In: "query", Description: "Maximum number of channels to return"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search_messages",
				Description: "Search messages",
				Method:      "GET",
				Path:        "/search.messages",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "query", Required: true, Description: "Search query string"},
					{Name: "count", In: "query", Description: "Number of results to return"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "channel_history",
				Description: "Get messages from a channel",
				Method:      "GET",
				Path:        "/conversations.history",
				Parameters: []config.ParamConfig{
					{Name: "channel", In: "query", Required: true, Description: "Channel ID"},
					{Name: "limit", In: "query", Description: "Maximum number of messages to return"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_users",
				Description: "List workspace members",
				Method:      "GET",
				Path:        "/users.list",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Maximum number of users to return"},
				},
				ActionType: "read",
			},
			{
				Name:        "add_reaction",
				Description: "Add a reaction to a message",
				Method:      "POST",
				Path:        "/reactions.add",
				Parameters: []config.ParamConfig{
					{Name: "channel", In: "body", Required: true, Description: "Channel ID"},
					{Name: "timestamp", In: "body", Required: true, Description: "Message timestamp"},
					{Name: "name", In: "body", Required: true, Description: "Emoji name (without colons)"},
				},
				ActionType: "write",
			},
		},
	}
}
