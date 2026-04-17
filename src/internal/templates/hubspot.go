// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/hubspot.yaml
var hubspotYAML string

// HubSpot returns the template for HubSpot.
func HubSpot() *Template {
	return &Template{
		Name:        "hubspot",
		DisplayName: "HubSpot",
		Description: "CRM, contacts, deals, and sales pipeline management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your private app token at https://app.hubspot.com/private-apps",
		VaultKey:    "HUBSPOT_API_KEY",
		YAML:        hubspotYAML,
	}
}
