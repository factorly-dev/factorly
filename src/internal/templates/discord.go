package templates

import _ "embed"

//go:embed yaml/discord.yaml
var discordYAML string

// Discord returns the template for Discord.
func Discord() *Template {
	return &Template{
		Name:        "discord",
		DisplayName: "Discord",
		Description: "Send messages and manage Discord servers via bot",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a bot at https://discord.com/developers/applications",
		VaultKey:    "DISCORD_BOT_TOKEN",
		YAML:        discordYAML,
	}
}
