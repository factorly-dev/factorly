package templates

import _ "embed"

//go:embed yaml/microsoft-outlook.yaml
var microsoftOutlookYAML string

// MicrosoftOutlook returns the template for Microsoft Outlook.
func MicrosoftOutlook() *Template {
	return &Template{
		Name:        "microsoft-outlook",
		DisplayName: "Microsoft Outlook",
		Description: "Email, calendar events, and contacts",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:   []string{"https://graph.microsoft.com/Mail.ReadWrite", "https://graph.microsoft.com/Mail.Send", "https://graph.microsoft.com/Calendars.ReadWrite"},
		},
		YAML: microsoftOutlookYAML,
	}
}
