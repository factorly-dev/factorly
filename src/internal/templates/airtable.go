package templates

import _ "embed"

//go:embed yaml/airtable.yaml
var airtableYAML string

// Airtable returns the template for Airtable.
func Airtable() *Template {
	return &Template{
		Name:        "airtable",
		DisplayName: "Airtable",
		Description: "Manage bases, tables, and records in Airtable",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://airtable.com/create/tokens",
		VaultKey:    "AIRTABLE_API_KEY",
		YAML:        airtableYAML,
	}
}
