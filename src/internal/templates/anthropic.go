// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/anthropic.yaml
var anthropicYAML string

// Anthropic returns the template for the Anthropic Claude API.
func Anthropic() *Template {
	return &Template{
		Name:        "anthropic",
		DisplayName: "Anthropic Claude API",
		Description: "Chat completions and text generation via the Claude API",
		Category:    "engineering",
		AuthType:    "api_key",
		AuthGuide:   "Get your API key at https://console.anthropic.com/settings/keys",
		VaultKey:    "ANTHROPIC_API_KEY",
		YAML:        anthropicYAML,
	}
}
