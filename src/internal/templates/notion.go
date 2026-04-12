package templates

import _ "embed"

//go:embed yaml/notion.yaml
var notionYAML string

// Notion returns the template for Notion.
func Notion() *Template {
	return &Template{
		Name:        "notion",
		DisplayName: "Notion",
		Description: "Workspace for docs, databases, and project management",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create an integration at https://www.notion.so/my-integrations",
		VaultKey:    "NOTION_API_KEY",
		YAML:        notionYAML,
	}
}
