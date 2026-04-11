package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// GoogleDocs returns the template for Google Docs document management.
func GoogleDocs() *Template {
	return &Template{
		Name:        "google-docs",
		DisplayName: "Google Docs",
		Description: "Create and read Google Docs documents",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "GOOGLE_API_TOKEN",
		BaseURL:     "https://docs.googleapis.com/v1",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "get_document",
				Description: "Get the content and metadata of a document",
				Method:      "GET",
				Path:        "/documents/{{documentId}}",
				Parameters: []config.ParamConfig{
					{Name: "documentId", In: "path", Required: true, Description: "The document ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_document",
				Description: "Create a new Google Docs document",
				Method:      "POST",
				Path:        "/documents",
				Parameters: []config.ParamConfig{
					{Name: "title", In: "body", Required: true, Description: "Document title"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "batch_update",
				Description: "Apply a batch of updates to a document",
				Method:      "POST",
				Path:        "/documents/{{documentId}}:batchUpdate",
				Parameters: []config.ParamConfig{
					{Name: "documentId", In: "path", Required: true, Description: "The document ID"},
					{Name: "requests", In: "body", Required: true, Description: "Array of update request objects"},
				},
				ActionType: "write",
			},
		},
	}
}
