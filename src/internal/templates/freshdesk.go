// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/freshdesk.yaml
var freshdeskYAML string

// Freshdesk returns the template for Freshdesk.
func Freshdesk() *Template {
	return &Template{
		Name:        "freshdesk",
		DisplayName: "Freshdesk",
		Description: "Help desk, ticket management, and customer support",
		Category:    "business",
		AuthType:    "api_key",
		AuthGuide:   "Find your API key at https://YOUR_DOMAIN.freshdesk.com/a/admin/api",
		VaultKey:    "FRESHDESK_API_KEY",
		YAML:        freshdeskYAML,
	}
}
