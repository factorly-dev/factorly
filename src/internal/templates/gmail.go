package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Gmail returns the template for Gmail email management.
func Gmail() *Template {
	return &Template{
		Name:        "gmail",
		DisplayName: "Gmail",
		Description: "Read, send, and manage email messages",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "GOOGLE_API_TOKEN",
		BaseURL:     "https://gmail.googleapis.com/gmail/v1",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_messages",
				Description: "List messages in the user's mailbox",
				Method:      "GET",
				Path:        "/users/me/messages",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Description: "Gmail search query (same as Gmail search box)"},
					{Name: "maxResults", In: "query", Description: "Maximum number of messages to return"},
					{Name: "labelIds", In: "query", Description: "Only return messages with these label IDs"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous list request"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_message",
				Description: "Get a specific email message",
				Method:      "GET",
				Path:        "/users/me/messages/{{messageId}}",
				Parameters: []config.ParamConfig{
					{Name: "messageId", In: "path", Required: true, Description: "The message ID"},
					{Name: "format", In: "query", Description: "Format to return (minimal, full, raw, metadata)"},
					{Name: "metadataHeaders", In: "query", Description: "Headers to include when format is metadata"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "send_message",
				Description: "Send an email message",
				Method:      "POST",
				Path:        "/users/me/messages/send",
				Parameters: []config.ParamConfig{
					{Name: "raw", In: "body", Required: true, Description: "Base64url encoded RFC 2822 email message"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search for email messages",
				Method:      "GET",
				Path:        "/users/me/messages",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "Gmail search query (e.g. 'from:user@example.com subject:hello')"},
					{Name: "maxResults", In: "query", Description: "Maximum number of messages to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous list request"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "create_draft",
				Description: "Create an email draft",
				Method:      "POST",
				Path:        "/users/me/drafts",
				Parameters: []config.ParamConfig{
					{Name: "raw", In: "body", Required: true, Description: "Base64url encoded RFC 2822 email message for the draft"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_labels",
				Description: "List all labels in the mailbox",
				Method:      "GET",
				Path:        "/users/me/labels",
				Parameters:  nil,
				ActionType:  "read",
			},
		},
	}
}
