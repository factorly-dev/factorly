// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/bigquery.yaml
var bigqueryYAML string

// BigQuery returns the template for Google BigQuery.
func BigQuery() *Template {
	return &Template{
		Name:        "bigquery",
		DisplayName: "Google BigQuery",
		Description: "Query and manage datasets and tables in BigQuery",
		Category:    "engineering",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/bigquery"},
		},
		YAML: bigqueryYAML,
	}
}
