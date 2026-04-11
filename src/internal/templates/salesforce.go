package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Salesforce returns the template for Salesforce CRM.
func Salesforce() *Template {
	return &Template{
		Name:        "salesforce",
		DisplayName: "Salesforce",
		Description: "Enterprise CRM, SOQL queries, and record management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your access token from Setup > Apps > Connected Apps",
		VaultKey:    "SALESFORCE_ACCESS_TOKEN",
		BaseURL:     "https://YOUR_INSTANCE.salesforce.com/services/data/v59.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "query",
				Description: "Execute a SOQL query",
				Method:      "GET",
				Path:        "/query",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "SOQL query string"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_record",
				Description: "Get a record by object type and ID",
				Method:      "GET",
				Path:        "/sobjects/{objectType}/{recordId}",
				Parameters: []config.ParamConfig{
					{Name: "objectType", In: "path", Required: true, Description: "SObject type (e.g. Account, Contact, Lead)"},
					{Name: "recordId", In: "path", Required: true, Description: "Record ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_record",
				Description: "Create a new record",
				Method:      "POST",
				Path:        "/sobjects/{objectType}",
				Parameters: []config.ParamConfig{
					{Name: "objectType", In: "path", Required: true, Description: "SObject type (e.g. Account, Contact, Lead)"},
					{Name: "body", In: "body", Required: true, Description: "Record fields as JSON"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search using SOSL",
				Method:      "GET",
				Path:        "/search",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "SOSL search string"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "update_record",
				Description: "Update an existing record",
				Method:      "PATCH",
				Path:        "/sobjects/{objectType}/{recordId}",
				Parameters: []config.ParamConfig{
					{Name: "objectType", In: "path", Required: true, Description: "SObject type"},
					{Name: "recordId", In: "path", Required: true, Description: "Record ID"},
					{Name: "body", In: "body", Required: true, Description: "Fields to update as JSON"},
				},
				ActionType: "write",
			},
			{
				Name:        "describe_object",
				Description: "Describe an SObject's metadata and fields",
				Method:      "GET",
				Path:        "/sobjects/{objectType}/describe",
				Parameters: []config.ParamConfig{
					{Name: "objectType", In: "path", Required: true, Description: "SObject type to describe"},
				},
				ActionType: "read",
			},
		},
	}
}
