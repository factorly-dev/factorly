package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Notion returns the template for Notion workspace management.
func Notion() *Template {
	return &Template{
		Name:        "notion",
		DisplayName: "Notion",
		Description: "Workspace for docs, databases, and project management",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create an integration at https://www.notion.so/my-integrations",
		VaultKey:    "NOTION_API_KEY",
		BaseURL:     "https://api.notion.com",
		Headers: map[string]string{
			"Content-Type":   "application/json",
			"Notion-Version": "2022-06-28",
		},
		Tools: []ToolDef{
			{
				Name:        "search",
				Description: "Search pages and databases",
				Method:      "POST",
				Path:        "/v1/search",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "Search query string"},
					{Name: "filter", In: "body", Description: "Filter by object type (page or database)"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_page",
				Description: "Get a page by ID",
				Method:      "GET",
				Path:        "/v1/pages/{{page_id}}",
				Parameters: []config.ParamConfig{
					{Name: "page_id", In: "path", Required: true, Description: "Notion page ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_page",
				Description: "Create a new page",
				Method:      "POST",
				Path:        "/v1/pages",
				Parameters: []config.ParamConfig{
					{Name: "parent", In: "body", Required: true, Description: "Parent page or database ID"},
					{Name: "title", In: "body", Required: true, Description: "Page title"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "query_database",
				Description: "Query a database",
				Method:      "POST",
				Path:        "/v1/databases/{{database_id}}/query",
				Parameters: []config.ParamConfig{
					{Name: "database_id", In: "path", Required: true, Description: "Database ID"},
					{Name: "filter", In: "body", Description: "Filter conditions"},
					{Name: "sorts", In: "body", Description: "Sort conditions"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_databases",
				Description: "List all databases",
				Method:      "POST",
				Path:        "/v1/search",
				Parameters: []config.ParamConfig{
					{Name: "filter", In: "body", Description: "Set to {\"property\":\"object\",\"value\":\"database\"}"},
				},
				ActionType: "read",
			},
			{
				Name:        "update_page",
				Description: "Update page properties",
				Method:      "PATCH",
				Path:        "/v1/pages/{{page_id}}",
				Parameters: []config.ParamConfig{
					{Name: "page_id", In: "path", Required: true, Description: "Notion page ID"},
					{Name: "properties", In: "body", Required: true, Description: "Properties to update"},
				},
				ActionType: "write",
			},
		},
	}
}
