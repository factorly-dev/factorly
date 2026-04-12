package templates

import _ "embed"

//go:embed yaml/calendly.yaml
var calendlyYAML string

// Calendly returns the template for Calendly.
func Calendly() *Template {
	return &Template{
		Name:        "calendly",
		DisplayName: "Calendly",
		Description: "Scheduling, events, and calendar management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://calendly.com/integrations/api_webhooks",
		VaultKey:    "CALENDLY_API_KEY",
		YAML:        calendlyYAML,
	}
}
