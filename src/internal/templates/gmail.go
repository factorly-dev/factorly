// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/gmail.yaml
var gmailYAML string

// Gmail returns the template for Gmail.
func Gmail() *Template {
	return &Template{
		Name:        "gmail",
		DisplayName: "Gmail",
		Description: "Read, send, and manage email messages",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/gmail.modify"},
		},
		YAML: gmailYAML,
	}
}
