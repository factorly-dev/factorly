package templates

import _ "embed"

//go:embed yaml/coda.yaml
var codaYAML string

// Coda returns the template for Coda.
func Coda() *Template {
	return &Template{
		Name:        "coda",
		DisplayName: "Coda",
		Description: "Manage docs, tables, and rows in Coda",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API key at https://coda.io/account",
		VaultKey:    "CODA_API_KEY",
		YAML:        codaYAML,
	}
}
