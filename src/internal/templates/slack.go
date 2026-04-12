package templates

import _ "embed"

//go:embed yaml/slack.yaml
var slackYAML string

// Slack returns the template for Slack.
func Slack() *Template {
	return &Template{
		Name:        "slack",
		DisplayName: "Slack",
		Description: "Team messaging, channels, and notifications",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a bot token at https://api.slack.com/apps (Bot User OAuth Token)",
		VaultKey:    "SLACK_BOT_TOKEN",
		YAML:        slackYAML,
	}
}
