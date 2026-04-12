package templates

import _ "embed"

//go:embed yaml/google-docs.yaml
var googleDocsYAML string

// GoogleDocs returns the template for Google Docs.
func GoogleDocs() *Template {
	return &Template{
		Name:        "google-docs",
		DisplayName: "Google Docs",
		Description: "Create and read Google Docs documents",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/documents"},
		},
		YAML: googleDocsYAML,
	}
}
