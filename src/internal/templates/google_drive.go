package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// GoogleDrive returns the template for Google Drive file management.
func GoogleDrive() *Template {
	return &Template{
		Name:        "google-drive",
		DisplayName: "Google Drive",
		Description: "Manage files and folders in Google Drive",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "", // OAuth uses provider-based auth, not a single vault key
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/drive"},
		},
		BaseURL:     "https://www.googleapis.com/drive/v3",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_files",
				Description: "List files in Google Drive",
				Method:      "GET",
				Path:        "/files",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Description: "Search query (Drive query syntax)"},
					{Name: "pageSize", In: "query", Description: "Maximum number of files to return (max 1000)"},
					{Name: "pageToken", In: "query", Description: "Page token from a previous list request"},
					{Name: "orderBy", In: "query", Description: "Sort order (e.g. 'modifiedTime desc')"},
					{Name: "fields", In: "query", Description: "Fields to include in the response"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_file",
				Description: "Get file metadata by ID",
				Method:      "GET",
				Path:        "/files/{{fileId}}",
				Parameters: []config.ParamConfig{
					{Name: "fileId", In: "path", Required: true, Description: "The file ID"},
					{Name: "fields", In: "query", Description: "Fields to include in the response"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search for files in Google Drive",
				Method:      "GET",
				Path:        "/files",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "Search query (e.g. \"name contains 'report'\")"},
					{Name: "pageSize", In: "query", Description: "Maximum number of files to return"},
					{Name: "fields", In: "query", Description: "Fields to include in the response"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "create_folder",
				Description: "Create a new folder in Google Drive",
				Method:      "POST",
				Path:        "/files",
				Parameters: []config.ParamConfig{
					{Name: "name", In: "body", Required: true, Description: "Folder name"},
					{Name: "mimeType", In: "body", Required: true, Description: "Must be 'application/vnd.google-apps.folder'"},
					{Name: "parents", In: "body", Description: "Array of parent folder IDs"},
				},
				ActionType: "write",
			},
			{
				Name:        "delete_file",
				Description: "Delete a file from Google Drive",
				Method:      "DELETE",
				Path:        "/files/{{fileId}}",
				Parameters: []config.ParamConfig{
					{Name: "fileId", In: "path", Required: true, Description: "The file ID to delete"},
				},
				ActionType: "delete",
			},
			{
				Name:        "share_file",
				Description: "Share a file by creating a permission",
				Method:      "POST",
				Path:        "/files/{{fileId}}/permissions",
				Parameters: []config.ParamConfig{
					{Name: "fileId", In: "path", Required: true, Description: "The file ID to share"},
					{Name: "role", In: "body", Required: true, Description: "Permission role (owner, organizer, writer, commenter, reader)"},
					{Name: "type", In: "body", Required: true, Description: "Permission type (user, group, domain, anyone)"},
					{Name: "emailAddress", In: "body", Description: "Email address for user or group type"},
				},
				ActionType: "write",
			},
		},
	}
}
