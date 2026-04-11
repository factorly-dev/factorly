package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Attio returns the template for Attio CRM.
func Attio() *Template {
	return &Template{
		Name:        "attio",
		DisplayName: "Attio",
		Description: "Modern CRM, records, and relationship management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API key at https://app.attio.com/settings/developers",
		VaultKey:    "ATTIO_API_KEY",
		BaseURL:     "https://api.attio.com/v2",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_records",
				Description: "List records for an object",
				Method:      "POST",
				Path:        "/objects/{object}/records/query",
				Parameters: []config.ParamConfig{
					{Name: "object", In: "path", Required: true, Description: "Object slug (e.g. people, companies)"},
					{Name: "limit", In: "body", Description: "Number of results"},
					{Name: "offset", In: "body", Description: "Offset for pagination"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_record",
				Description: "Create a new record",
				Method:      "POST",
				Path:        "/objects/{object}/records",
				Parameters: []config.ParamConfig{
					{Name: "object", In: "path", Required: true, Description: "Object slug (e.g. people, companies)"},
					{Name: "data", In: "body", Required: true, Description: "Record data with attribute values"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search records with filters",
				Method:      "POST",
				Path:        "/objects/{object}/records/query",
				Parameters: []config.ParamConfig{
					{Name: "object", In: "path", Required: true, Description: "Object slug"},
					{Name: "filter", In: "body", Required: true, Description: "Filter conditions"},
					{Name: "limit", In: "body", Description: "Number of results"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "update_record",
				Description: "Update an existing record",
				Method:      "PUT",
				Path:        "/objects/{object}/records/{record_id}",
				Parameters: []config.ParamConfig{
					{Name: "object", In: "path", Required: true, Description: "Object slug"},
					{Name: "record_id", In: "path", Required: true, Description: "Record ID"},
					{Name: "data", In: "body", Required: true, Description: "Record fields to update"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_objects",
				Description: "List available objects in the workspace",
				Method:      "GET",
				Path:        "/objects",
				Parameters:  nil,
				ActionType:  "read",
			},
			{
				Name:        "get_record",
				Description: "Get a record by ID",
				Method:      "GET",
				Path:        "/objects/{object}/records/{record_id}",
				Parameters: []config.ParamConfig{
					{Name: "object", In: "path", Required: true, Description: "Object slug"},
					{Name: "record_id", In: "path", Required: true, Description: "Record ID"},
				},
				ActionType: "read",
			},
		},
	}
}
