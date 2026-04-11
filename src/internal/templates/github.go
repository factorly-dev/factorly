package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// GitHub returns the template for GitHub code hosting and collaboration.
func GitHub() *Template {
	return &Template{
		Name:        "github",
		DisplayName: "GitHub",
		Description: "Code hosting, issues, pull requests, and repositories",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a token at https://github.com/settings/tokens",
		VaultKey:    "GITHUB_TOKEN",
		BaseURL:     "https://api.github.com",
		Headers: map[string]string{
			"Accept":     "application/vnd.github.v3+json",
			"User-Agent": "factorly",
		},
		Tools: []ToolDef{
			{
				Name:        "list_repos",
				Description: "List repositories for a user",
				Method:      "GET",
				Path:        "/users/{{username}}/repos",
				Parameters: []config.ParamConfig{
					{Name: "username", In: "path", Required: true, Description: "GitHub username"},
					{Name: "sort", In: "query", Description: "Sort field (created, updated, pushed, full_name)"},
					{Name: "per_page", In: "query", Description: "Results per page (max 100)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_repo",
				Description: "Get repository details",
				Method:      "GET",
				Path:        "/repos/{{owner}}/{{repo}}",
				Parameters: []config.ParamConfig{
					{Name: "owner", In: "path", Required: true, Description: "Repository owner"},
					{Name: "repo", In: "path", Required: true, Description: "Repository name"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_issues",
				Description: "List issues for a repository",
				Method:      "GET",
				Path:        "/repos/{{owner}}/{{repo}}/issues",
				Parameters: []config.ParamConfig{
					{Name: "owner", In: "path", Required: true, Description: "Repository owner"},
					{Name: "repo", In: "path", Required: true, Description: "Repository name"},
					{Name: "state", In: "query", Description: "State filter (open, closed, all)"},
					{Name: "labels", In: "query", Description: "Comma-separated label names"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_issue",
				Description: "Create an issue",
				Method:      "POST",
				Path:        "/repos/{{owner}}/{{repo}}/issues",
				Parameters: []config.ParamConfig{
					{Name: "owner", In: "path", Required: true, Description: "Repository owner"},
					{Name: "repo", In: "path", Required: true, Description: "Repository name"},
					{Name: "title", In: "body", Required: true, Description: "Issue title"},
					{Name: "body", In: "body", Description: "Issue body (markdown)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_prs",
				Description: "List pull requests",
				Method:      "GET",
				Path:        "/repos/{{owner}}/{{repo}}/pulls",
				Parameters: []config.ParamConfig{
					{Name: "owner", In: "path", Required: true, Description: "Repository owner"},
					{Name: "repo", In: "path", Required: true, Description: "Repository name"},
					{Name: "state", In: "query", Description: "State filter (open, closed, all)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "search_code",
				Description: "Search code across repositories",
				Method:      "GET",
				Path:        "/search/code",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "Search query"},
					{Name: "per_page", In: "query", Description: "Results per page (max 100)"},
				},
				ActionType: "search",
			},
			{
				Name:        "search_issues",
				Description: "Search issues and pull requests",
				Method:      "GET",
				Path:        "/search/issues",
				Parameters: []config.ParamConfig{
					{Name: "q", In: "query", Required: true, Description: "Search query"},
					{Name: "sort", In: "query", Description: "Sort field (comments, reactions, created, updated)"},
					{Name: "per_page", In: "query", Description: "Results per page (max 100)"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "get_user",
				Description: "Get the authenticated user's profile",
				Method:      "GET",
				Path:        "/user",
				Parameters:  nil,
				ActionType:  "read",
			},
		},
	}
}
