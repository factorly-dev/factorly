package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Freshdesk returns the template for Freshdesk help desk.
func Freshdesk() *Template {
	return &Template{
		Name:        "freshdesk",
		DisplayName: "Freshdesk",
		Description: "Help desk, ticket management, and customer support",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Find your API key at https://YOUR_DOMAIN.freshdesk.com/a/admin/api",
		VaultKey:    "FRESHDESK_API_KEY",
		BaseURL:     "https://YOUR_DOMAIN.freshdesk.com/api/v2",
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
					{Name: "filter", In: "query", Description: "Filter name (new_and_my_open, watched, spam, deleted)"},
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
					{Name: "subject", In: "body", Required: true, Description: "Ticket subject"},
					{Name: "description", In: "body", Required: true, Description: "Ticket description (HTML)"},
					{Name: "email", In: "body", Required: true, Description: "Requester email"},
					{Name: "priority", In: "body", Description: "Priority (1=Low, 2=Medium, 3=High, 4=Urgent)"},
					{Name: "status", In: "body", Description: "Status (2=Open, 3=Pending, 4=Resolved, 5=Closed)"},
				},
				ActionType: "write",
				Essential:  true,
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
				Essential:  true,
			},
			{
				Name:        "update_ticket",
				Description: "Update an existing ticket",
				Method:      "PUT",
				Path:        "/tickets/{ticket_id}",
				Parameters: []config.ParamConfig{
					{Name: "ticket_id", In: "path", Required: true, Description: "Ticket ID"},
					{Name: "status", In: "body", Description: "Status"},
					{Name: "priority", In: "body", Description: "Priority"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_contacts",
				Description: "List contacts",
				Method:      "GET",
				Path:        "/contacts",
				Parameters: []config.ParamConfig{
					{Name: "page", In: "query", Description: "Page number"},
					{Name: "per_page", In: "query", Description: "Results per page"},
				},
				ActionType: "read",
			},
			{
				Name:        "search_tickets",
				Description: "Search tickets",
				Method:      "GET",
				Path:        "/search/tickets",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "query", Required: true, Description: "Search query string (Freshdesk query format)"},
				},
				ActionType: "search",
			},
		},
	}
}
