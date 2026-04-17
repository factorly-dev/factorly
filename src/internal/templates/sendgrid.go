// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/sendgrid.yaml
var sendgridYAML string

// SendGrid returns the template for SendGrid.
func SendGrid() *Template {
	return &Template{
		Name:        "sendgrid",
		DisplayName: "SendGrid",
		Description: "Email delivery, contacts, and marketing campaigns",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create an API key at https://app.sendgrid.com/settings/api_keys",
		VaultKey:    "SENDGRID_API_KEY",
		YAML:        sendgridYAML,
	}
}
