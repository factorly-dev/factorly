package templates

import _ "embed"

//go:embed yaml/trello.yaml
var trelloYAML string

// Trello returns the template for Trello.
func Trello() *Template {
	return &Template{
		Name:        "trello",
		DisplayName: "Trello",
		Description: "Kanban boards, cards, and task management",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://trello.com/power-ups/admin",
		VaultKey:    "TRELLO_API_KEY",
		YAML:        trelloYAML,
	}
}
