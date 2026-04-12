package templates

import _ "embed"

//go:embed yaml/salesforce.yaml
var salesforceYAML string

// Salesforce returns the template for Salesforce.
func Salesforce() *Template {
	return &Template{
		Name:        "salesforce",
		DisplayName: "Salesforce",
		Description: "Enterprise CRM, SOQL queries, and record management",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create a Connected App at Setup > App Manager",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://login.salesforce.com/services/oauth2/authorize",
			TokenURL: "https://login.salesforce.com/services/oauth2/token",
			Scopes:   []string{"api", "refresh_token"},
		},
		YAML: salesforceYAML,
	}
}
