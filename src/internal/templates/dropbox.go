// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/dropbox.yaml
var dropboxYAML string

// Dropbox returns the template for Dropbox.
func Dropbox() *Template {
	return &Template{
		Name:        "dropbox",
		DisplayName: "Dropbox",
		Description: "Cloud file storage, sharing, and collaboration",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create an app at https://www.dropbox.com/developers/apps",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://www.dropbox.com/oauth2/authorize",
			TokenURL: "https://api.dropboxapi.com/oauth2/token",
			Scopes:   []string{"files.content.read", "files.content.write"},
		},
		YAML: dropboxYAML,
	}
}
