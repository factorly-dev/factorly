package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Jira returns the template for Jira issue tracking.
func Jira() *Template {
	return &Template{
		Name:        "jira",
		DisplayName: "Jira",
		Description: "Manage issues, projects, and workflows in Jira",
		Category:    "engineering",
		AuthType:    "api_key",
		AuthGuide:   "Create an API token at https://id.atlassian.com/manage-profile/security/api-tokens",
		VaultKey:    "JIRA_API_TOKEN",
		BaseURL:     "https://YOUR_DOMAIN.atlassian.net/rest/api/3",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_issues",
				Description: "List issues using JQL",
				Method:      "GET",
				Path:        "/search",
				Parameters: []config.ParamConfig{
					{Name: "jql", In: "query", Description: "JQL query string"},
					{Name: "maxResults", In: "query", Description: "Maximum number of issues to return"},
					{Name: "startAt", In: "query", Description: "Index of the first result to return"},
					{Name: "fields", In: "query", Description: "Comma-separated list of fields to return"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_issue",
				Description: "Get a specific issue by key or ID",
				Method:      "GET",
				Path:        "/issue/{{issueIdOrKey}}",
				Parameters: []config.ParamConfig{
					{Name: "issueIdOrKey", In: "path", Required: true, Description: "Issue key (e.g. PROJ-123) or ID"},
					{Name: "fields", In: "query", Description: "Comma-separated list of fields to return"},
					{Name: "expand", In: "query", Description: "Fields to expand (e.g. changelog, renderedFields)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_issue",
				Description: "Create a new Jira issue",
				Method:      "POST",
				Path:        "/issue",
				Parameters: []config.ParamConfig{
					{Name: "project", In: "body", Required: true, Description: "Project object with key (e.g. {\"key\": \"PROJ\"})"},
					{Name: "summary", In: "body", Required: true, Description: "Issue summary/title"},
					{Name: "issuetype", In: "body", Required: true, Description: "Issue type object (e.g. {\"name\": \"Task\"})"},
					{Name: "description", In: "body", Description: "Issue description (Atlassian Document Format)"},
					{Name: "assignee", In: "body", Description: "Assignee object with accountId"},
					{Name: "priority", In: "body", Description: "Priority object with name"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search for issues using JQL",
				Method:      "POST",
				Path:        "/search",
				Parameters: []config.ParamConfig{
					{Name: "jql", In: "body", Required: true, Description: "JQL query string"},
					{Name: "maxResults", In: "body", Description: "Maximum number of issues to return"},
					{Name: "startAt", In: "body", Description: "Index of the first result to return"},
					{Name: "fields", In: "body", Description: "Array of field names to return"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "transition_issue",
				Description: "Transition an issue to a new status",
				Method:      "POST",
				Path:        "/issue/{{issueIdOrKey}}/transitions",
				Parameters: []config.ParamConfig{
					{Name: "issueIdOrKey", In: "path", Required: true, Description: "Issue key (e.g. PROJ-123) or ID"},
					{Name: "transition", In: "body", Required: true, Description: "Transition object with id (e.g. {\"id\": \"31\"})"},
				},
				ActionType: "write",
			},
			{
				Name:        "add_comment",
				Description: "Add a comment to an issue",
				Method:      "POST",
				Path:        "/issue/{{issueIdOrKey}}/comment",
				Parameters: []config.ParamConfig{
					{Name: "issueIdOrKey", In: "path", Required: true, Description: "Issue key (e.g. PROJ-123) or ID"},
					{Name: "body", In: "body", Required: true, Description: "Comment body (Atlassian Document Format)"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_projects",
				Description: "List all accessible projects",
				Method:      "GET",
				Path:        "/project",
				Parameters: []config.ParamConfig{
					{Name: "maxResults", In: "query", Description: "Maximum number of projects to return"},
					{Name: "startAt", In: "query", Description: "Index of the first result to return"},
				},
				ActionType: "read",
			},
		},
	}
}
