package templates

import _ "embed"

//go:embed yaml/apollo.yaml
var apolloYAML string

// Apollo returns the template for Apollo.
func Apollo() *Template {
	return &Template{
		Name:        "apollo",
		DisplayName: "Apollo",
		Description: "Sales intelligence, prospecting, and contact enrichment",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://app.apollo.io/settings/integrations/api",
		VaultKey:    "APOLLO_API_KEY",
		YAML:        apolloYAML,
	}
}
