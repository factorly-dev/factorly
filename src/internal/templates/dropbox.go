package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Dropbox returns the template for Dropbox file storage.
func Dropbox() *Template {
	return &Template{
		Name:        "dropbox",
		DisplayName: "Dropbox",
		Description: "Cloud file storage, sharing, and collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create an app at https://www.dropbox.com/developers/apps",
		VaultKey:    "DROPBOX_ACCESS_TOKEN",
		BaseURL:     "https://api.dropboxapi.com/2",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_folder",
				Description: "List files and folders in a path",
				Method:      "POST",
				Path:        "/files/list_folder",
				Parameters: []config.ParamConfig{
					{Name: "path", In: "body", Required: true, Description: "Folder path (empty string for root)"},
					{Name: "recursive", In: "body", Description: "Recursively list contents (true/false)"},
					{Name: "limit", In: "body", Description: "Maximum number of results"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search for files and folders",
				Method:      "POST",
				Path:        "/files/search_v2",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "Search query string"},
					{Name: "options", In: "body", Description: "Search options (path, max_results, file_extensions, etc.)"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_metadata",
				Description: "Get metadata for a file or folder",
				Method:      "POST",
				Path:        "/files/get_metadata",
				Parameters: []config.ParamConfig{
					{Name: "path", In: "body", Required: true, Description: "File or folder path"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_folder",
				Description: "Create a new folder",
				Method:      "POST",
				Path:        "/files/create_folder_v2",
				Parameters: []config.ParamConfig{
					{Name: "path", In: "body", Required: true, Description: "Folder path to create"},
					{Name: "autorename", In: "body", Description: "Auto-rename if conflict (true/false)"},
				},
				ActionType: "write",
			},
			{
				Name:        "delete",
				Description: "Delete a file or folder",
				Method:      "POST",
				Path:        "/files/delete_v2",
				Parameters: []config.ParamConfig{
					{Name: "path", In: "body", Required: true, Description: "Path to delete"},
				},
				ActionType: "delete",
			},
			{
				Name:        "move",
				Description: "Move a file or folder",
				Method:      "POST",
				Path:        "/files/move_v2",
				Parameters: []config.ParamConfig{
					{Name: "from_path", In: "body", Required: true, Description: "Source path"},
					{Name: "to_path", In: "body", Required: true, Description: "Destination path"},
					{Name: "autorename", In: "body", Description: "Auto-rename if conflict (true/false)"},
				},
				ActionType: "write",
			},
		},
	}
}
