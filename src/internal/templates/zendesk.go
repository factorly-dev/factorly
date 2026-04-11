package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Zendesk returns the template for Zendesk customer support.
func Zendesk() *Template {
	return &Template{
		Name:        "zendesk",
		DisplayName: "Zendesk",
		Description: "Customer support tickets, users, and help desk",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://YOUR_SUBDOMAIN.zendesk.com/admin/apps-integrations/apis/zendesk-api/settings",
		VaultKey:    "ZENDESK_API_TOKEN",
		BaseURL:     "https://YOUR_SUBDOMAIN.zendesk.com/api/v2",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_tickets",
				Description: "List tickets",
				Method:      "GET",
				Path:        "/tickets",
				Parameters: []config.ParamConfig{
					{Name: "page", In: "query", Description: "Page number"},
					{Name: "per_page", In: "query", Description: "Results per page (1-100)"},
					{Name: "sort_by", In: "query", Description: "Sort field (created_at, updated_at, etc.)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_ticket",
				Description: "Create a new ticket",
				Method:      "POST",
				Path:        "/tickets",
				Parameters: []config.ParamConfig{
					{Name: "ticket", In: "body", Required: true, Description: "Ticket object with subject, description, priority, etc."},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search across tickets, users, and organizations",
				Method:      "GET",
				Path:        "/search",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "query", Required: true, Description: "Zendesk search query string"},
					{Name: "page", In: "query", Description: "Page number"},
					{Name: "per_page", In: "query", Description: "Results per page"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "update_ticket",
				Description: "Update an existing ticket",
				Method:      "PUT",
				Path:        "/tickets/{ticket_id}",
				Parameters: []config.ParamConfig{
					{Name: "ticket_id", In: "path", Required: true, Description: "Ticket ID"},
					{Name: "ticket", In: "body", Required: true, Description: "Ticket fields to update"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_ticket",
				Description: "Get a ticket by ID",
				Method:      "GET",
				Path:        "/tickets/{ticket_id}",
				Parameters: []config.ParamConfig{
					{Name: "ticket_id", In: "path", Required: true, Description: "Ticket ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "list_users",
				Description: "List users",
				Method:      "GET",
				Path:        "/users",
				Parameters: []config.ParamConfig{
					{Name: "page", In: "query", Description: "Page number"},
					{Name: "per_page", In: "query", Description: "Results per page"},
				},
				ActionType: "read",
			},
		},
	}
}
