// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/linear.yaml
var linearYAML string

// Linear returns the template for Linear.
func Linear() *Template {
	return &Template{
		Name:        "linear",
		DisplayName: "Linear",
		Description: "Issue tracking and project management",
		Category:    "engineering",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://linear.app/settings/api",
		VaultKey:    "LINEAR_API_KEY",
		YAML:        linearYAML,
	}
}
