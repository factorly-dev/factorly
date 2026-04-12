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
		AuthType:    "bearer",
		AuthGuide:   "Create a bot with @BotFather on Telegram — it will give you the token",
		VaultKey:    "TELEGRAM_BOT_TOKEN",
		YAML:        telegramYAML,
	}
}
