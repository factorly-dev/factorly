// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/onedrive.yaml
var onedriveYAML string

// OneDrive returns the template for OneDrive.
func OneDrive() *Template {
	return &Template{
		Name:        "onedrive",
		DisplayName: "OneDrive",
		Description: "Cloud file storage, sharing, and document management",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:   []string{"https://graph.microsoft.com/Files.ReadWrite.All"},
		},
		YAML: onedriveYAML,
	}
}
