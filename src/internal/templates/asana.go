package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Asana returns the template for Asana project management.
func Asana() *Template {
	return &Template{
		Name:        "asana",
		DisplayName: "Asana",
		Description: "Project management, tasks, and team collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create a token at https://app.asana.com/0/developer-console",
		VaultKey:    "ASANA_ACCESS_TOKEN",
		BaseURL:     "https://app.asana.com/api/1.0",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_tasks",
				Description: "List tasks in a project",
				Method:      "GET",
				Path:        "/tasks",
				Parameters: []config.ParamConfig{
					{Name: "project", In: "query", Required: true, Description: "Project GID"},
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "completed_since", In: "query", Description: "Filter tasks completed after this time"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_task",
				Description: "Create a new task",
				Method:      "POST",
				Path:        "/tasks",
				Parameters: []config.ParamConfig{
					{Name: "data", In: "body", Required: true, Description: "Task data (name, projects, assignee, notes, due_on, etc.)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "search",
				Description: "Search tasks in a workspace",
				Method:      "GET",
				Path:        "/workspaces/{workspace_gid}/tasks/search",
				Parameters: []config.ParamConfig{
					{Name: "workspace_gid", In: "path", Required: true, Description: "Workspace GID"},
					{Name: "text", In: "query", Description: "Search text"},
					{Name: "completed", In: "query", Description: "Filter by completion (true/false)"},
				},
				ActionType: "search",
				Essential:  true,
			},
			{
				Name:        "list_projects",
				Description: "List projects in a workspace",
				Method:      "GET",
				Path:        "/projects",
				Parameters: []config.ParamConfig{
					{Name: "workspace", In: "query", Required: true, Description: "Workspace GID"},
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_task",
				Description: "Update an existing task",
				Method:      "PUT",
				Path:        "/tasks/{task_gid}",
				Parameters: []config.ParamConfig{
					{Name: "task_gid", In: "path", Required: true, Description: "Task GID"},
					{Name: "data", In: "body", Required: true, Description: "Task fields to update"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_task",
				Description: "Get a task by GID",
				Method:      "GET",
				Path:        "/tasks/{task_gid}",
				Parameters: []config.ParamConfig{
					{Name: "task_gid", In: "path", Required: true, Description: "Task GID"},
				},
				ActionType: "read",
			},
		},
	}
}
