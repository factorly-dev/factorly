package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Apollo returns the template for Apollo sales intelligence.
func Apollo() *Template {
	return &Template{
		Name:        "apollo",
		DisplayName: "Apollo",
		Description: "Sales intelligence, prospecting, and contact enrichment",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://app.apollo.io/settings/integrations/api",
		VaultKey:    "APOLLO_API_KEY",
		BaseURL:     "https://api.apollo.io/api/v1",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "search_people",
				Description: "Search for people/contacts",
				Method:      "POST",
				Path:        "/mixed_people/search",
				Parameters: []config.ParamConfig{
					{Name: "q_keywords", In: "body", Description: "Keywords to search for"},
					{Name: "person_titles", In: "body", Description: "Array of job titles to filter by"},
					{Name: "person_locations", In: "body", Description: "Array of locations to filter by"},
					{Name: "page", In: "body", Description: "Page number"},
					{Name: "per_page", In: "body", Description: "Results per page"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "enrich_person",
				Description: "Enrich a person's data by email or domain",
				Method:      "POST",
				Path:        "/people/match",
				Parameters: []config.ParamConfig{
					{Name: "email", In: "body", Description: "Email address to enrich"},
					{Name: "first_name", In: "body", Description: "First name"},
					{Name: "last_name", In: "body", Description: "Last name"},
					{Name: "domain", In: "body", Description: "Company domain"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search_organizations",
				Description: "Search for organizations/companies",
				Method:      "POST",
				Path:        "/mixed_companies/search",
				Parameters: []config.ParamConfig{
					{Name: "q_keywords", In: "body", Description: "Keywords to search for"},
					{Name: "organization_locations", In: "body", Description: "Array of locations"},
					{Name: "page", In: "body", Description: "Page number"},
					{Name: "per_page", In: "body", Description: "Results per page"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "create_contact",
				Description: "Create a contact in Apollo",
				Method:      "POST",
				Path:        "/contacts",
				Parameters: []config.ParamConfig{
					{Name: "first_name", In: "body", Required: true, Description: "First name"},
					{Name: "last_name", In: "body", Required: true, Description: "Last name"},
					{Name: "email", In: "body", Description: "Email address"},
					{Name: "organization_name", In: "body", Description: "Company name"},
					{Name: "title", In: "body", Description: "Job title"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_sequences",
				Description: "List email sequences",
				Method:      "GET",
				Path:        "/emailer_campaigns",
				Parameters: []config.ParamConfig{
					{Name: "page", In: "query", Description: "Page number"},
					{Name: "per_page", In: "query", Description: "Results per page"},
				},
				ActionType: "read",
			},
		},
	}
}
