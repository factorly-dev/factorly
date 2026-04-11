package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Airtable returns the template for Airtable database management.
func Airtable() *Template {
	return &Template{
		Name:        "airtable",
		DisplayName: "Airtable",
		Description: "Manage bases, tables, and records in Airtable",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://airtable.com/create/tokens",
		VaultKey:    "AIRTABLE_API_KEY",
		BaseURL:     "https://api.airtable.com/v0",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_records",
				Description: "List records in a table",
				Method:      "GET",
				Path:        "/{{baseId}}/{{tableIdOrName}}",
				Parameters: []config.ParamConfig{
					{Name: "baseId", In: "path", Required: true, Description: "The Airtable base ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "maxRecords", In: "query", Description: "Maximum number of records to return"},
					{Name: "view", In: "query", Description: "Name or ID of a view to filter by"},
					{Name: "filterByFormula", In: "query", Description: "Airtable formula to filter records"},
					{Name: "sort", In: "query", Description: "Sort configuration"},
					{Name: "offset", In: "query", Description: "Pagination offset from a previous request"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_record",
				Description: "Get a single record by ID",
				Method:      "GET",
				Path:        "/{{baseId}}/{{tableIdOrName}}/{{recordId}}",
				Parameters: []config.ParamConfig{
					{Name: "baseId", In: "path", Required: true, Description: "The Airtable base ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "recordId", In: "path", Required: true, Description: "The record ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_record",
				Description: "Create a new record in a table",
				Method:      "POST",
				Path:        "/{{baseId}}/{{tableIdOrName}}",
				Parameters: []config.ParamConfig{
					{Name: "baseId", In: "path", Required: true, Description: "The Airtable base ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "fields", In: "body", Required: true, Description: "Object of field names to values"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "update_record",
				Description: "Update an existing record",
				Method:      "PATCH",
				Path:        "/{{baseId}}/{{tableIdOrName}}/{{recordId}}",
				Parameters: []config.ParamConfig{
					{Name: "baseId", In: "path", Required: true, Description: "The Airtable base ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "recordId", In: "path", Required: true, Description: "The record ID"},
					{Name: "fields", In: "body", Required: true, Description: "Object of field names to updated values"},
				},
				ActionType: "write",
			},
			{
				Name:        "delete_record",
				Description: "Delete a record from a table",
				Method:      "DELETE",
				Path:        "/{{baseId}}/{{tableIdOrName}}/{{recordId}}",
				Parameters: []config.ParamConfig{
					{Name: "baseId", In: "path", Required: true, Description: "The Airtable base ID"},
					{Name: "tableIdOrName", In: "path", Required: true, Description: "The table ID or name"},
					{Name: "recordId", In: "path", Required: true, Description: "The record ID to delete"},
				},
				ActionType: "delete",
			},
			{
				Name:        "list_bases",
				Description: "List all accessible bases",
				Method:      "GET",
				Path:        "/meta/bases",
				Parameters: []config.ParamConfig{
					{Name: "offset", In: "query", Description: "Pagination offset"},
				},
				ActionType: "read",
			},
		},
	}
}
