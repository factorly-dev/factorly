package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// HubSpot returns the template for HubSpot CRM.
func HubSpot() *Template {
	return &Template{
		Name:        "hubspot",
		DisplayName: "HubSpot",
		Description: "CRM, contacts, deals, and sales pipeline management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your private app token at https://app.hubspot.com/private-apps",
		VaultKey:    "HUBSPOT_API_KEY",
		BaseURL:     "https://api.hubapi.com",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_contacts",
				Description: "List contacts",
				Method:      "GET",
				Path:        "/crm/v3/objects/contacts",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "after", In: "query", Description: "Cursor for pagination"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_contact",
				Description: "Create a new contact",
				Method:      "POST",
				Path:        "/crm/v3/objects/contacts",
				Parameters: []config.ParamConfig{
					{Name: "properties", In: "body", Required: true, Description: "Contact properties JSON (email, firstname, lastname, etc.)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_deals",
				Description: "List deals",
				Method:      "GET",
				Path:        "/crm/v3/objects/deals",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "after", In: "query", Description: "Cursor for pagination"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search across CRM objects",
				Method:      "POST",
				Path:        "/crm/v3/objects/contacts/search",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "Search query string"},
					{Name: "limit", In: "body", Description: "Number of results"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_contact",
				Description: "Get a contact by ID",
				Method:      "GET",
				Path:        "/crm/v3/objects/contacts/{contactId}",
				Parameters: []config.ParamConfig{
					{Name: "contactId", In: "path", Required: true, Description: "Contact ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "create_deal",
				Description: "Create a new deal",
				Method:      "POST",
				Path:        "/crm/v3/objects/deals",
				Parameters: []config.ParamConfig{
					{Name: "properties", In: "body", Required: true, Description: "Deal properties JSON (dealname, amount, dealstage, etc.)"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_companies",
				Description: "List companies",
				Method:      "GET",
				Path:        "/crm/v3/objects/companies",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "after", In: "query", Description: "Cursor for pagination"},
				},
				ActionType: "read",
			},
		},
	}
}
