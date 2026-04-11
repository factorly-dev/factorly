package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// MicrosoftTeams returns the template for Microsoft Teams.
func MicrosoftTeams() *Template {
	return &Template{
		Name:        "microsoft-teams",
		DisplayName: "Microsoft Teams",
		Description: "Team messaging, channels, and collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		VaultKey:    "MICROSOFT_ACCESS_TOKEN",
		BaseURL:     "https://graph.microsoft.com/v1.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "send_message",
				Description: "Send a message to a channel",
				Method:      "POST",
				Path:        "/teams/{team_id}/channels/{channel_id}/messages",
				Parameters: []config.ParamConfig{
					{Name: "team_id", In: "path", Required: true, Description: "Team ID"},
					{Name: "channel_id", In: "path", Required: true, Description: "Channel ID"},
					{Name: "body", In: "body", Required: true, Description: "Message body object (content, contentType)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_channels",
				Description: "List channels in a team",
				Method:      "GET",
				Path:        "/teams/{team_id}/channels",
				Parameters: []config.ParamConfig{
					{Name: "team_id", In: "path", Required: true, Description: "Team ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_teams",
				Description: "List joined teams",
				Method:      "GET",
				Path:        "/me/joinedTeams",
				Parameters:  nil,
				ActionType:  "read",
				Essential:   true,
			},
			{
				Name:        "list_members",
				Description: "List members of a team",
				Method:      "GET",
				Path:        "/teams/{team_id}/members",
				Parameters: []config.ParamConfig{
					{Name: "team_id", In: "path", Required: true, Description: "Team ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "get_channel",
				Description: "Get a channel by ID",
				Method:      "GET",
				Path:        "/teams/{team_id}/channels/{channel_id}",
				Parameters: []config.ParamConfig{
					{Name: "team_id", In: "path", Required: true, Description: "Team ID"},
					{Name: "channel_id", In: "path", Required: true, Description: "Channel ID"},
				},
				ActionType: "read",
			},
		},
	}
}
