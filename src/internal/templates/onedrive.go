package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// OneDrive returns the template for Microsoft OneDrive.
func OneDrive() *Template {
	return &Template{
		Name:        "onedrive",
		DisplayName: "OneDrive",
		Description: "Cloud file storage, sharing, and document management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		VaultKey:    "MICROSOFT_ACCESS_TOKEN",
		BaseURL:     "https://graph.microsoft.com/v1.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_files",
				Description: "List files in a folder",
				Method:      "GET",
				Path:        "/me/drive/root/children",
				Parameters: []config.ParamConfig{
					{Name: "$top", In: "query", Description: "Number of results"},
					{Name: "$orderby", In: "query", Description: "Sort order"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_file",
				Description: "Get file metadata by ID",
				Method:      "GET",
				Path:        "/me/drive/items/{item_id}",
				Parameters: []config.ParamConfig{
					{Name: "item_id", In: "path", Required: true, Description: "Drive item ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search for files",
				Method:      "GET",
				Path:        "/me/drive/root/search(q='{query}')",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "path", Required: true, Description: "Search query string"},
					{Name: "$top", In: "query", Description: "Number of results"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "create_folder",
				Description: "Create a new folder",
				Method:      "POST",
				Path:        "/me/drive/root/children",
				Parameters: []config.ParamConfig{
					{Name: "name", In: "body", Required: true, Description: "Folder name"},
					{Name: "folder", In: "body", Required: true, Description: "Empty folder object ({})"},
				},
				ActionType: "write",
			},
			{
				Name:        "upload_file",
				Description: "Upload a small file (up to 4MB)",
				Method:      "PUT",
				Path:        "/me/drive/root:/{filename}:/content",
				Parameters: []config.ParamConfig{
					{Name: "filename", In: "path", Required: true, Description: "File path and name"},
					{Name: "body", In: "body", Required: true, Description: "File content"},
				},
				ActionType: "write",
			},
		},
	}
}
