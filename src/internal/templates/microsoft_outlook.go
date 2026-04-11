package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// MicrosoftOutlook returns the template for Microsoft Outlook.
func MicrosoftOutlook() *Template {
	return &Template{
		Name:        "microsoft-outlook",
		DisplayName: "Microsoft Outlook",
		Description: "Email, calendar events, and contacts",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		VaultKey:    "MICROSOFT_ACCESS_TOKEN",
		BaseURL:     "https://graph.microsoft.com/v1.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_messages",
				Description: "List email messages",
				Method:      "GET",
				Path:        "/me/messages",
				Parameters: []config.ParamConfig{
					{Name: "$top", In: "query", Description: "Number of results"},
					{Name: "$filter", In: "query", Description: "OData filter expression"},
					{Name: "$orderby", In: "query", Description: "Sort order (e.g. receivedDateTime desc)"},
					{Name: "$select", In: "query", Description: "Fields to return"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "send_mail",
				Description: "Send an email",
				Method:      "POST",
				Path:        "/me/sendMail",
				Parameters: []config.ParamConfig{
					{Name: "message", In: "body", Required: true, Description: "Message object (subject, body, toRecipients)"},
					{Name: "saveToSentItems", In: "body", Description: "Save to sent items (true/false)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_events",
				Description: "List calendar events",
				Method:      "GET",
				Path:        "/me/events",
				Parameters: []config.ParamConfig{
					{Name: "$top", In: "query", Description: "Number of results"},
					{Name: "$filter", In: "query", Description: "OData filter expression"},
					{Name: "$orderby", In: "query", Description: "Sort order"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_message",
				Description: "Get an email message by ID",
				Method:      "GET",
				Path:        "/me/messages/{message_id}",
				Parameters: []config.ParamConfig{
					{Name: "message_id", In: "path", Required: true, Description: "Message ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "create_event",
				Description: "Create a calendar event",
				Method:      "POST",
				Path:        "/me/events",
				Parameters: []config.ParamConfig{
					{Name: "subject", In: "body", Required: true, Description: "Event subject"},
					{Name: "start", In: "body", Required: true, Description: "Start time object (dateTime, timeZone)"},
					{Name: "end", In: "body", Required: true, Description: "End time object (dateTime, timeZone)"},
					{Name: "attendees", In: "body", Description: "Array of attendee objects"},
				},
				ActionType: "write",
			},
			{
				Name:        "search_messages",
				Description: "Search email messages",
				Method:      "GET",
				Path:        "/me/messages",
				Parameters: []config.ParamConfig{
					{Name: "$search", In: "query", Required: true, Description: "Search query string"},
					{Name: "$top", In: "query", Description: "Number of results"},
				},
				ActionType: "search",
			},
		},
	}
}
