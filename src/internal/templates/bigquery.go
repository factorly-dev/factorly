package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// BigQuery returns the template for Google BigQuery data warehouse.
func BigQuery() *Template {
	return &Template{
		Name:        "bigquery",
		DisplayName: "Google BigQuery",
		Description: "Query and manage datasets and tables in BigQuery",
		Category:    "engineering",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "", // OAuth uses provider-based auth, not a single vault key
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/bigquery"},
		},
		BaseURL:     "https://bigquery.googleapis.com/bigquery/v2",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "query",
				Description: "Run a BigQuery SQL query",
				Method:      "POST",
				Path:        "/projects/{{projectId}}/queries",
				Parameters: []config.ParamConfig{
					{Name: "projectId", In: "path", Required: true, Description: "Google Cloud project ID"},
					{Name: "query", In: "body", Required: true, Description: "SQL query string"},
					{Name: "useLegacySql", In: "body", Description: "Whether to use legacy SQL (default false)"},
					{Name: "maxResults", In: "body", Description: "Maximum number of rows to return"},
					{Name: "timeoutMs", In: "body", Description: "Query timeout in milliseconds"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_tables",
				Description: "List tables in a dataset",
				Method:      "GET",
				Path:        "/projects/{{projectId}}/datasets/{{datasetId}}/tables",
				Parameters: []config.ParamConfig{
					{Name: "projectId", In: "path", Required: true, Description: "Google Cloud project ID"},
					{Name: "datasetId", In: "path", Required: true, Description: "Dataset ID"},
					{Name: "maxResults", In: "query", Description: "Maximum number of tables to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous request"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_datasets",
				Description: "List datasets in a project",
				Method:      "GET",
				Path:        "/projects/{{projectId}}/datasets",
				Parameters: []config.ParamConfig{
					{Name: "projectId", In: "path", Required: true, Description: "Google Cloud project ID"},
					{Name: "maxResults", In: "query", Description: "Maximum number of datasets to return"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous request"},
					{Name: "all", In: "query", Description: "Whether to list all datasets including hidden ones"},
				},
				ActionType: "read",
			},
			{
				Name:        "get_table",
				Description: "Get metadata for a specific table",
				Method:      "GET",
				Path:        "/projects/{{projectId}}/datasets/{{datasetId}}/tables/{{tableId}}",
				Parameters: []config.ParamConfig{
					{Name: "projectId", In: "path", Required: true, Description: "Google Cloud project ID"},
					{Name: "datasetId", In: "path", Required: true, Description: "Dataset ID"},
					{Name: "tableId", In: "path", Required: true, Description: "Table ID"},
				},
				ActionType: "read",
			},
		},
	}
}
