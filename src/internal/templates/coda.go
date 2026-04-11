package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Coda returns the template for the Coda document platform.
func Coda() *Template {
	return &Template{
		Name:        "coda",
		DisplayName: "Coda",
		Description: "Manage docs, tables, and rows in Coda",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API key at https://coda.io/account",
		VaultKey:    "CODA_API_KEY",
		BaseURL:     "https://coda.io/apis/v1",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_docs",
				Description: "List available Coda docs",
				Method:      "GET",
				Path:        "/docs",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "query", Description: "Search term to filter docs by name"},
					{Name: "limit", In: "query", Description: "Maximum number of docs to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous request"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_doc",
				Description: "Get metadata for a specific doc",
				Method:      "GET",
				Path:        "/docs/{{docId}}",
				Parameters: []config.ParamConfig{
					{Name: "docId", In: "path", Required: true, Description: "The doc ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_rows",
				Description: "List rows in a table",
				Method:      "GET",
				Path:        "/docs/{{docId}}/tables/{{tableIdOrName}}/rows",
				Parameters: []config.ParamConfig{
					{Name: "docId", In: "path", Required: true, Description: "The doc ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "query", In: "query", Description: "Search query to filter rows"},
					{Name: "limit", In: "query", Description: "Maximum number of rows to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous request"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "upsert_row",
				Description: "Insert or update rows in a table",
				Method:      "POST",
				Path:        "/docs/{{docId}}/tables/{{tableIdOrName}}/rows",
				Parameters: []config.ParamConfig{
					{Name: "docId", In: "path", Required: true, Description: "The doc ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "rows", In: "body", Required: true, Description: "Array of row objects with cells to upsert"},
					{Name: "keyColumns", In: "body", Description: "Array of column IDs to use as upsert keys"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_tables",
				Description: "List tables in a doc",
				Method:      "GET",
				Path:        "/docs/{{docId}}/tables",
				Parameters: []config.ParamConfig{
					{Name: "docId", In: "path", Required: true, Description: "The doc ID"},
					{Name: "limit", In: "query", Description: "Maximum number of tables to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous request"},
				},
				ActionType: "read",
			},
		},
	}
}
