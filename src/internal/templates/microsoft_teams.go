// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/microsoft-teams.yaml
var microsoftTeamsYAML string

// MicrosoftTeams returns the template for Microsoft Teams.
func MicrosoftTeams() *Template {
	return &Template{
		Name:        "microsoft-teams",
		DisplayName: "Microsoft Teams",
		Description: "Team messaging, channels, and collaboration",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Register an app at https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:   []string{"https://graph.microsoft.com/Chat.ReadWrite", "https://graph.microsoft.com/Channel.ReadBasic.All"},
		},
		YAML: microsoftTeamsYAML,
	}
}
