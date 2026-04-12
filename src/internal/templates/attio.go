package templates

import _ "embed"

//go:embed yaml/attio.yaml
var attioYAML string

// Attio returns the template for Attio.
func Attio() *Template {
	return &Template{
		Name:        "attio",
		DisplayName: "Attio",
		Description: "Modern CRM, records, and relationship management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API key at https://app.attio.com/settings/developers",
		VaultKey:    "ATTIO_API_KEY",
		YAML:        attioYAML,
	}
}
