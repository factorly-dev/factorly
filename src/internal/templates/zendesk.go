package templates

import _ "embed"

//go:embed yaml/zendesk.yaml
var zendeskYAML string

// Zendesk returns the template for Zendesk.
func Zendesk() *Template {
	return &Template{
		Name:        "zendesk",
		DisplayName: "Zendesk",
		Description: "Customer support tickets, users, and help desk",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://YOUR_SUBDOMAIN.zendesk.com/admin/apps-integrations/apis/zendesk-api/settings",
		VaultKey:    "ZENDESK_API_TOKEN",
		YAML:        zendeskYAML,
	}
}
