// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/chatgpt.yaml
var chatgptYAML string

// ChatGPT returns the template for the OpenAI ChatGPT API.
func ChatGPT() *Template {
	return &Template{
		Name:        "chatgpt",
		DisplayName: "OpenAI ChatGPT API",
		Description: "Chat completions via the OpenAI API",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Get your API key at https://platform.openai.com/api-keys",
		VaultKey:    "OPENAI_API_KEY",
		YAML:        chatgptYAML,
	}
}
