package templates

import _ "embed"

//go:embed yaml/intercom.yaml
var intercomYAML string

// Intercom returns the template for Intercom.
func Intercom() *Template {
	return &Template{
		Name:        "intercom",
		DisplayName: "Intercom",
		Description: "Customer messaging, conversations, and support",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://app.intercom.com/a/apps/_/developer-hub",
		VaultKey:    "INTERCOM_ACCESS_TOKEN",
		YAML:        intercomYAML,
	}
}
