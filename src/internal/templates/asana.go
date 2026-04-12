package templates

import _ "embed"

//go:embed yaml/asana.yaml
var asanaYAML string

// Asana returns the template for Asana.
func Asana() *Template {
	return &Template{
		Name:        "asana",
		DisplayName: "Asana",
		Description: "Project management, tasks, and team collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create a token at https://app.asana.com/0/developer-console",
		VaultKey:    "ASANA_ACCESS_TOKEN",
		YAML:        asanaYAML,
	}
}
