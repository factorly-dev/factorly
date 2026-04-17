// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/sharepoint.yaml
var sharepointYAML string

// SharePoint returns the template for SharePoint.
func SharePoint() *Template {
	return &Template{
		Name:        "sharepoint",
		DisplayName: "SharePoint",
		Description: "Document management, sites, and team collaboration",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:   []string{"https://graph.microsoft.com/Sites.ReadWrite.All"},
		},
		YAML: sharepointYAML,
	}
}
