package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Linear returns the template for Linear issue tracking.
func Linear() *Template {
	return &Template{
		Name:        "linear",
		DisplayName: "Linear",
		Description: "Issue tracking and project management",
		Category:    "engineering",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://linear.app/settings/api",
		VaultKey:    "LINEAR_API_KEY",
		BaseURL:     "https://api.linear.app",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_issues",
				Description: "List issues assigned to you",
				Method:      "POST",
				Path:        "/graphql",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Description: "GraphQL query (default: viewer's issues)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_issue",
				Description: "Get issue details by identifier",
				Method:      "POST",
				Path:        "/graphql",
				Parameters: []config.ParamConfig{
					{Name: "id", In: "body", Required: true, Description: "Issue identifier (e.g., ENG-123)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_issue",
				Description: "Create a new issue",
				Method:      "POST",
				Path:        "/graphql",
				Parameters: []config.ParamConfig{
					{Name: "title", In: "body", Required: true, Description: "Issue title"},
					{Name: "teamId", In: "body", Required: true, Description: "Team ID"},
					{Name: "description", In: "body", Description: "Issue description (markdown)"},
					{Name: "priority", In: "body", Description: "Priority (0-4)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "update_issue",
				Description: "Update an existing issue",
				Method:      "POST",
				Path:        "/graphql",
				Parameters: []config.ParamConfig{
					{Name: "id", In: "body", Required: true, Description: "Issue ID"},
					{Name: "title", In: "body", Description: "New title"},
					{Name: "state", In: "body", Description: "New state"},
					{Name: "assigneeId", In: "body", Description: "Assignee user ID"},
				},
				ActionType: "write",
			},
			{
				Name:        "search",
				Description: "Search issues, projects, and documents",
				Method:      "POST",
				Path:        "/graphql",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "Search query string"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "list_teams",
				Description: "List all teams in the workspace",
				Method:      "POST",
				Path:        "/graphql",
				Parameters:  nil,
				ActionType:  "read",
			},
		},
	}
}
