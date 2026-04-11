package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// SharePoint returns the template for Microsoft SharePoint.
func SharePoint() *Template {
	return &Template{
		Name:        "sharepoint",
		DisplayName: "SharePoint",
		Description: "Document management, sites, and team collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		VaultKey:    "MICROSOFT_ACCESS_TOKEN",
		BaseURL:     "https://graph.microsoft.com/v1.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_sites",
				Description: "List SharePoint sites",
				Method:      "GET",
				Path:        "/sites",
				Parameters: []config.ParamConfig{
					{Name: "search", In: "query", Description: "Search query to filter sites"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_files",
				Description: "List files in a site's default document library",
				Method:      "GET",
				Path:        "/sites/{site_id}/drive/root/children",
				Parameters: []config.ParamConfig{
					{Name: "site_id", In: "path", Required: true, Description: "Site ID"},
					{Name: "$top", In: "query", Description: "Number of results"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search across SharePoint",
				Method:      "POST",
				Path:        "/search/query",
				Parameters: []config.ParamConfig{
					{Name: "requests", In: "body", Required: true, Description: "Array of search request objects (queryString, entityTypes)"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_site",
				Description: "Get a site by ID",
				Method:      "GET",
				Path:        "/sites/{site_id}",
				Parameters: []config.ParamConfig{
					{Name: "site_id", In: "path", Required: true, Description: "Site ID or hostname:path"},
				},
				ActionType: "read",
			},
			{
				Name:        "create_list",
				Description: "Create a new list in a site",
				Method:      "POST",
				Path:        "/sites/{site_id}/lists",
				Parameters: []config.ParamConfig{
					{Name: "site_id", In: "path", Required: true, Description: "Site ID"},
					{Name: "displayName", In: "body", Required: true, Description: "List display name"},
					{Name: "list", In: "body", Required: true, Description: "List template object"},
				},
				ActionType: "write",
			},
		},
	}
}
