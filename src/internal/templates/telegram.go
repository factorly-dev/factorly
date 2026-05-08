// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/telegram.yaml
var telegramYAML string

// Telegram returns the template for Telegram.
func Telegram() *Template {
	return &Template{
		Name:        "telegram",
		DisplayName: "Telegram",
		Description: "Bot messaging, channels, and notifications via phone",
		Category:    "engineering",
		AuthType:    "vault",
		AuthGuide:   "Create a bot with @BotFather on Telegram, then run: factorly vault set TELEGRAM_BOT_TOKEN <token>",
		VaultKey:    "TELEGRAM_BOT_TOKEN",
		YAML:        telegramYAML,
	}
}
