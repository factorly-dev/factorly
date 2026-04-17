// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/google-drive.yaml
var googleDriveYAML string

// GoogleDrive returns the template for Google Drive.
func GoogleDrive() *Template {
	return &Template{
		Name:        "google-drive",
		DisplayName: "Google Drive",
		Description: "Manage files and folders in Google Drive",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/drive"},
		},
		YAML: googleDriveYAML,
	}
}
