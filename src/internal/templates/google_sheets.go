// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/google-sheets.yaml
var googleSheetsYAML string

// GoogleSheets returns the template for Google Sheets.
func GoogleSheets() *Template {
	return &Template{
		Name:        "google-sheets",
		DisplayName: "Google Sheets",
		Description: "Read and write Google Sheets spreadsheets",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/spreadsheets"},
		},
		YAML: googleSheetsYAML,
	}
}
