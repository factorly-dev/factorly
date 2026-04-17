// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/docusign.yaml
var docusignYAML string

// DocuSign returns the template for DocuSign.
func DocuSign() *Template {
	return &Template{
		Name:        "docusign",
		DisplayName: "DocuSign",
		Description: "Electronic signatures, envelopes, and document management",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create an app at https://developers.docusign.com",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://account-d.docusign.com/oauth/auth",
			TokenURL: "https://account-d.docusign.com/oauth/token",
			Scopes:   []string{"signature"},
		},
		YAML: docusignYAML,
	}
}
