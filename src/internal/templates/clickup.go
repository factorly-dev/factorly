package templates

import _ "embed"

//go:embed yaml/clickup.yaml
var clickupYAML string

// ClickUp returns the template for ClickUp.
func ClickUp() *Template {
	return &Template{
		Name:        "clickup",
		DisplayName: "ClickUp",
		Description: "Project management, tasks, and productivity",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API token at https://app.clickup.com/settings/apps",
		VaultKey:    "CLICKUP_API_KEY",
		YAML:        clickupYAML,
	}
}
