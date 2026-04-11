package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Monday returns the template for monday.com work management.
func Monday() *Template {
	return &Template{
		Name:        "monday",
		DisplayName: "monday.com",
		Description: "Work management, boards, and team collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API token at https://YOUR_DOMAIN.monday.com/apps/manage/tokens",
		VaultKey:    "MONDAY_API_KEY",
		BaseURL:     "https://api.monday.com/v2",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_items",
				Description: "List items on a board via GraphQL",
				Method:      "POST",
				Path:        "",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "GraphQL query string (e.g. { boards(ids: 123) { items_page { items { id name } } } })"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_item",
				Description: "Create a new item on a board via GraphQL",
				Method:      "POST",
				Path:        "",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "GraphQL mutation (e.g. mutation { create_item(board_id: 123, item_name: \"New\") { id } })"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_boards",
				Description: "List boards via GraphQL",
				Method:      "POST",
				Path:        "",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "GraphQL query (e.g. { boards { id name } })"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_item",
				Description: "Update an item via GraphQL",
				Method:      "POST",
				Path:        "",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "GraphQL mutation to update item column values"},
				},
				ActionType: "write",
			},
			{
				Name:        "search",
				Description: "Search items via GraphQL",
				Method:      "POST",
				Path:        "",
				Parameters: []config.ParamConfig{
					{Name: "query", In: "body", Required: true, Description: "GraphQL query with items_page_by_column_values or similar search"},
				},
				ActionType: "search",
			},
		},
	}
}
