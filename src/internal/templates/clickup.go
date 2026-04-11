package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// ClickUp returns the template for ClickUp project management.
func ClickUp() *Template {
	return &Template{
		Name:        "clickup",
		DisplayName: "ClickUp",
		Description: "Project management, tasks, and productivity",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API token at https://app.clickup.com/settings/apps",
		VaultKey:    "CLICKUP_API_KEY",
		BaseURL:     "https://api.clickup.com/api/v2",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_tasks",
				Description: "List tasks in a list",
				Method:      "GET",
				Path:        "/list/{list_id}/task",
				Parameters: []config.ParamConfig{
					{Name: "list_id", In: "path", Required: true, Description: "List ID"},
					{Name: "page", In: "query", Description: "Page number (starts at 0)"},
					{Name: "subtasks", In: "query", Description: "Include subtasks (true/false)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_task",
				Description: "Create a new task",
				Method:      "POST",
				Path:        "/list/{list_id}/task",
				Parameters: []config.ParamConfig{
					{Name: "list_id", In: "path", Required: true, Description: "List ID"},
					{Name: "name", In: "body", Required: true, Description: "Task name"},
					{Name: "description", In: "body", Description: "Task description"},
					{Name: "priority", In: "body", Description: "Priority (1=Urgent, 2=High, 3=Normal, 4=Low)"},
					{Name: "assignees", In: "body", Description: "Array of assignee user IDs"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "get_task",
				Description: "Get a task by ID",
				Method:      "GET",
				Path:        "/task/{task_id}",
				Parameters: []config.ParamConfig{
					{Name: "task_id", In: "path", Required: true, Description: "Task ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_task",
				Description: "Update an existing task",
				Method:      "PUT",
				Path:        "/task/{task_id}",
				Parameters: []config.ParamConfig{
					{Name: "task_id", In: "path", Required: true, Description: "Task ID"},
					{Name: "name", In: "body", Description: "Task name"},
					{Name: "description", In: "body", Description: "Task description"},
					{Name: "status", In: "body", Description: "Task status"},
					{Name: "priority", In: "body", Description: "Priority"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_spaces",
				Description: "List spaces in a team",
				Method:      "GET",
				Path:        "/team/{team_id}/space",
				Parameters: []config.ParamConfig{
					{Name: "team_id", In: "path", Required: true, Description: "Team (workspace) ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "list_lists",
				Description: "List lists in a folder",
				Method:      "GET",
				Path:        "/folder/{folder_id}/list",
				Parameters: []config.ParamConfig{
					{Name: "folder_id", In: "path", Required: true, Description: "Folder ID"},
				},
				ActionType: "read",
			},
		},
	}
}
