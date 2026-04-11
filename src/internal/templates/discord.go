package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Discord returns the template for Discord bot interactions.
func Discord() *Template {
	return &Template{
		Name:        "discord",
		DisplayName: "Discord",
		Description: "Send messages and manage Discord servers via bot",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a bot at https://discord.com/developers/applications",
		VaultKey:    "DISCORD_BOT_TOKEN",
		BaseURL:     "https://discord.com/api/v10",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bot {{vault:DISCORD_BOT_TOKEN}}",
		},
		Tools: []ToolDef{
			{
				Name:        "send_message",
				Description: "Send a message to a channel",
				Method:      "POST",
				Path:        "/channels/{{channelId}}/messages",
				Parameters: []config.ParamConfig{
					{Name: "channelId", In: "path", Required: true, Description: "The channel ID"},
					{Name: "content", In: "body", Required: true, Description: "Message content (up to 2000 characters)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_channels",
				Description: "List channels in a guild",
				Method:      "GET",
				Path:        "/guilds/{{guildId}}/channels",
				Parameters: []config.ParamConfig{
					{Name: "guildId", In: "path", Required: true, Description: "The guild (server) ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_guild_members",
				Description: "List members of a guild",
				Method:      "GET",
				Path:        "/guilds/{{guildId}}/members",
				Parameters: []config.ParamConfig{
					{Name: "guildId", In: "path", Required: true, Description: "The guild (server) ID"},
					{Name: "limit", In: "query", Description: "Max number of members to return (1-1000, default 1)"},
					{Name: "after", In: "query", Description: "Get members after this user ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_reaction",
				Description: "Add a reaction to a message",
				Method:      "PUT",
				Path:        "/channels/{{channelId}}/messages/{{messageId}}/reactions/{{emoji}}/@me",
				Parameters: []config.ParamConfig{
					{Name: "channelId", In: "path", Required: true, Description: "The channel ID"},
					{Name: "messageId", In: "path", Required: true, Description: "The message ID"},
					{Name: "emoji", In: "path", Required: true, Description: "URL-encoded emoji (e.g. %F0%9F%91%8D for thumbs up)"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_channel",
				Description: "Get a channel by ID",
				Method:      "GET",
				Path:        "/channels/{{channelId}}",
				Parameters: []config.ParamConfig{
					{Name: "channelId", In: "path", Required: true, Description: "The channel ID"},
				},
				ActionType: "read",
			},
		},
	}
}
