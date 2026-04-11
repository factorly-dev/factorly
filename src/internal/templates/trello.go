package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Trello returns the template for Trello boards and cards.
func Trello() *Template {
	return &Template{
		Name:        "trello",
		DisplayName: "Trello",
		Description: "Kanban boards, cards, and task management",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://trello.com/power-ups/admin",
		VaultKey:    "TRELLO_API_KEY",
		BaseURL:     "https://api.trello.com/1",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_cards",
				Description: "List cards on a board",
				Method:      "GET",
				Path:        "/boards/{board_id}/cards",
				Parameters: []config.ParamConfig{
					{Name: "board_id", In: "path", Required: true, Description: "Board ID"},
					{Name: "filter", In: "query", Description: "Filter (all, closed, none, open, visible)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_card",
				Description: "Create a new card",
				Method:      "POST",
				Path:        "/cards",
				Parameters: []config.ParamConfig{
					{Name: "idList", In: "query", Required: true, Description: "List ID to add the card to"},
					{Name: "name", In: "query", Required: true, Description: "Card name"},
					{Name: "desc", In: "query", Description: "Card description"},
					{Name: "pos", In: "query", Description: "Position (top, bottom, or a number)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_boards",
				Description: "List boards for the authenticated member",
				Method:      "GET",
				Path:        "/members/me/boards",
				Parameters: []config.ParamConfig{
					{Name: "filter", In: "query", Description: "Filter (all, closed, members, open, starred)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_card",
				Description: "Update a card",
				Method:      "PUT",
				Path:        "/cards/{card_id}",
				Parameters: []config.ParamConfig{
					{Name: "card_id", In: "path", Required: true, Description: "Card ID"},
					{Name: "name", In: "query", Description: "Card name"},
					{Name: "desc", In: "query", Description: "Card description"},
					{Name: "closed", In: "query", Description: "Archive card (true/false)"},
				},
				ActionType: "write",
			},
			{
				Name:        "move_card",
				Description: "Move a card to a different list",
				Method:      "PUT",
				Path:        "/cards/{card_id}",
				Parameters: []config.ParamConfig{
					{Name: "card_id", In: "path", Required: true, Description: "Card ID"},
					{Name: "idList", In: "query", Required: true, Description: "Target list ID"},
					{Name: "pos", In: "query", Description: "Position in list (top, bottom)"},
				},
				ActionType: "write",
			},
			{
				Name:        "list_lists",
				Description: "List lists on a board",
				Method:      "GET",
				Path:        "/boards/{board_id}/lists",
				Parameters: []config.ParamConfig{
					{Name: "board_id", In: "path", Required: true, Description: "Board ID"},
					{Name: "filter", In: "query", Description: "Filter (all, closed, none, open)"},
				},
				ActionType: "read",
			},
		},
	}
}
