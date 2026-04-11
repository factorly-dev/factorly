package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Intercom returns the template for Intercom customer messaging.
func Intercom() *Template {
	return &Template{
		Name:        "intercom",
		DisplayName: "Intercom",
		Description: "Customer messaging, conversations, and support",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://app.intercom.com/a/apps/_/developer-hub",
		VaultKey:    "INTERCOM_ACCESS_TOKEN",
		BaseURL:     "https://api.intercom.io",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_conversations",
				Description: "List conversations",
				Method:      "GET",
				Path:        "/conversations",
				Parameters: []config.ParamConfig{
					{Name: "per_page", In: "query", Description: "Number of results per page"},
					{Name: "starting_after", In: "query", Description: "Cursor for pagination"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search_contacts",
				Description: "Search for contacts",
				Method:      "POST",
				Path:        "/contacts/search",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "Search query object"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "reply_conversation",
				Description: "Reply to a conversation",
				Method:      "POST",
				Path:        "/conversations/{conversation_id}/reply",
				Parameters: []config.ParamConfig{
					{Name: "conversation_id", In: "path", Required: true, Description: "Conversation ID"},
					{Name: "message_type", In: "body", Required: true, Description: "Type of reply (comment, note)"},
					{Name: "type", In: "body", Required: true, Description: "Sender type (admin, user)"},
					{Name: "body", In: "body", Required: true, Description: "Reply body text"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "create_contact",
				Description: "Create a new contact",
				Method:      "POST",
				Path:        "/contacts",
				Parameters: []config.ParamConfig{
					{Name: "role", In: "body", Required: true, Description: "Contact role (user, lead)"},
					{Name: "email", In: "body", Description: "Contact email"},
					{Name: "name", In: "body", Description: "Contact name"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_contacts",
				Description: "List contacts",
				Method:      "GET",
				Path:        "/contacts",
				Parameters: []config.ParamConfig{
					{Name: "per_page", In: "query", Description: "Number of results per page"},
					{Name: "starting_after", In: "query", Description: "Cursor for pagination"},
				},
				ActionType: "read",
			},
			{
				Name:        "get_conversation",
				Description: "Get a conversation by ID",
				Method:      "GET",
				Path:        "/conversations/{conversation_id}",
				Parameters: []config.ParamConfig{
					{Name: "conversation_id", In: "path", Required: true, Description: "Conversation ID"},
				},
				ActionType: "read",
			},
		},
	}
}
