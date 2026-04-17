// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/monday.yaml
var mondayYAML string

// Monday returns the template for monday.com.
func Monday() *Template {
	return &Template{
		Name:        "monday",
		DisplayName: "monday.com",
		Description: "Work management, boards, and team collaboration",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your API token at https://YOUR_DOMAIN.monday.com/apps/manage/tokens",
		VaultKey:    "MONDAY_API_KEY",
		YAML:        mondayYAML,
	}
}
